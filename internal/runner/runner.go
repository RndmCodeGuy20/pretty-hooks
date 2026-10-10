// Package runner executes hook steps with fail-fast semantics.
package runner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/RndmCodeGuy20/pretty-hooks/internal/cache"
	"github.com/RndmCodeGuy20/pretty-hooks/internal/config"
	"github.com/RndmCodeGuy20/pretty-hooks/internal/render"
	"github.com/RndmCodeGuy20/pretty-hooks/internal/stats"
)

// tailLines caps how much of a failed step's output the summary replays.
const tailLines = 20

// Options tunes a Run.
type Options struct {
	// Skip names steps to skip (from --skip flags).
	Skip map[string]bool
	// Staged lists repo-relative staged files for pre-commit filtering.
	Staged []string
	// FilterChanged enables only_if_changed filtering (pre-commit).
	// When false, all steps run regardless of OnlyIfChanged (pre-push).
	FilterChanged bool
	// Fix runs each failing step's fix command once, re-stages repaired
	// files, and runs the step one final time.
	Fix bool
	// Hook names the running hook. Used for cache storage paths.
	Hook string
	// NoCache bypasses cache reads and writes.
	NoCache bool
	// Stash hides unstaged changes while steps run. The stash is restored
	// on every exit path; a failed restore overrides the run error.
	Stash bool
	// RootDir is the working directory default (repo root). Empty means cwd.
	RootDir string
}

// FailedError is returned when a step fails (fail-fast: first failure wins).
type FailedError struct {
	Step string
	Code int
}

func (e *FailedError) Error() string {
	return fmt.Sprintf("step %q failed (exit %d)", e.Step, e.Code)
}

// Run executes steps in order, streaming progress to r.
// It stops at the first failure and returns the results plus a *FailedError.
func Run(steps []config.Step, r *render.Renderer, out io.Writer, opts Options) ([]render.Result, error) {
	return run(steps, r, out, opts)
}

func run(steps []config.Step, r *render.Renderer, out io.Writer, opts Options) (results []render.Result, runErr error) {
	if opts.Stash {
		stashed, err := stashPushIfDirty(opts.RootDir)
		if err != nil {
			return nil, err
		}
		if stashed {
			defer func() {
				if perr := stashPop(opts.RootDir); perr != nil {
					runErr = perr
				}
			}()
		}
	}
	results = make([]render.Result, 0, len(steps))
	for i := 0; i < len(steps); {
		j := i + 1
		// Only named groups merge; empty groups always run sequentially.
		name := ""
		if steps[i].Group != "" {
			name = steps[i].Group
			for j < len(steps) && steps[j].Group == name {
				j++
			}
		}
		gres, gerr := runGroup(steps[i:j], name, i, len(steps), opts, r)
		results = append(results, gres...)
		if gerr != nil {
			return results, gerr
		}
		i = j
	}
	return results, nil
}

// runGroup runs one step directly or a named group concurrently.
// Members of a group all run (failures collected); any failure stops
// later groups via the returned error.
func runGroup(group []config.Step, name string, offset, total int, opts Options, r *render.Renderer) ([]render.Result, error) {
	if name == "" {
		res, err := runOne(group[0], offset, total, opts, r, func(line string) { r.StepLine(line) })
		return []render.Result{res}, err
	}
	type outcome struct {
		res render.Result
		err error
	}
	outcomes := make([]outcome, len(group))
	var wg sync.WaitGroup
	for k, s := range group {
		wg.Add(1)
		go func() {
			defer wg.Done()
			emit := func(line string) { r.StepLine(s.Name + " | " + line) }
			res, err := runOne(s, offset+k, total, opts, r, emit)
			outcomes[k] = outcome{res, err}
		}()
	}
	wg.Wait()
	results := make([]render.Result, 0, len(group))
	var firstErr error
	for _, o := range outcomes {
		results = append(results, o.res)
		if o.err != nil && firstErr == nil {
			firstErr = o.err
		}
	}
	return results, firstErr
}

func runOne(s config.Step, i, total int, opts Options, r *render.Renderer, emit func(string)) (render.Result, error) {
	if opts.Skip[s.Name] {
		r.StepSkipped(s.Name, "--skip")
		return render.Result{Name: s.Name, Status: render.Skipped, Reason: "--skip"}, nil
	}
	if len(s.OnlyIfChanged) > 0 && opts.FilterChanged && !matchAny(s.OnlyIfChanged, opts.Staged) {
		r.StepSkipped(s.Name, "no matching changed files")
		return render.Result{Name: s.Name, Status: render.Skipped, Reason: "no matching changed files"}, nil
	}
	key, cacheable := cacheKey(s, opts)
	if cacheable && !opts.NoCache {
		if h, ok := cache.Load(opts.RootDir, opts.Hook, s.Name); ok && h == key {
			r.StepCached(s.Name)
			return render.Result{Name: s.Name, Status: render.Cached}, nil
		}
	}
	r.StepBegin(i+1, total, s.Name, s.Run)
	start := time.Now()
	tail, code, err := execStep(s, opts.RootDir, opts.Staged, emit)
	dur := time.Since(start)
	if err != nil && opts.Fix && s.Fix != "" {
		tail, code, dur, err = attemptFix(s, i, total, opts, r, emit)
	}
	if err != nil {
		reason := fmt.Sprintf("exit %d", code)
		if code < 0 {
			reason = err.Error()
		}
		r.StepFail(s.Name, reason, dur)
		stats.Append(opts.RootDir, opts.Hook, s.Name, dur, "fail")
		return render.Result{Name: s.Name, Status: render.Fail, Duration: dur, Reason: reason, Tail: tail}, &FailedError{Step: s.Name, Code: code}
	}
	r.StepOK(s.Name, dur)
	stats.Append(opts.RootDir, opts.Hook, s.Name, dur, "pass")
	if cacheable && !opts.NoCache {
		if err := cache.Store(opts.RootDir, opts.Hook, s.Name, key); err != nil {
			r.Warn(fmt.Sprintf("cache store failed for %q: %v", s.Name, err))
		}
	}
	return render.Result{Name: s.Name, Status: render.Pass, Duration: dur}, nil
}

// execStep runs one step via 'sh -c', streaming each output line to emit and
// keeping the last tailLines lines for failure summaries.
func execStep(s config.Step, root string, staged []string, emit func(string)) ([]string, int, error) {
	cmd := exec.Command("sh", "-c", expandPlaceholders(s.Run, s, staged))
	dir := root
	if s.Workdir != "" {
		dir = s.Workdir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(root, dir)
		}
	}
	if dir != "" {
		cmd.Dir = dir
	}
	if len(s.Env) > 0 {
		env := os.Environ()
		for k, v := range s.Env {
			env = append(env, k+"="+os.ExpandEnv(v))
		}
		cmd.Env = env
	}
	stream := &lineStream{emit: emit, keep: tailLines}
	cmd.Stdout = stream
	cmd.Stderr = stream
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return stream.tail(), ee.ExitCode(), ee
		}
		return stream.tail(), -1, err
	}
	return nil, 0, nil
}

// attemptFix runs the step's fix command, re-stages repaired files, and runs
// the step exactly once more. It returns the final attempt's outcome.
func attemptFix(s config.Step, i, total int, opts Options, r *render.Renderer, emit func(string)) ([]string, int, time.Duration, error) {
	r.Info(fmt.Sprintf("fix: %s", s.Fix))
	fixStep := config.Step{Name: s.Name + " fix", Run: s.Fix, Workdir: s.Workdir, Env: s.Env}
	_, _, _ = execStep(fixStep, opts.RootDir, opts.Staged, emit)
	if err := restage(s, opts); err != nil {
		return nil, -1, 0, fmt.Errorf("restage: %w", err)
	}
	r.StepBegin(i+1, total, s.Name, s.Run)
	start := time.Now()
	tail, code, err := execStep(s, opts.RootDir, opts.Staged, emit)
	return tail, code, time.Since(start), err
}

// restage re-adds the step's files that still exist on disk.
// It only ever adds; it never unstages.
func restage(s config.Step, opts Options) error {
	var existing []string
	for _, f := range stagedList(s, opts.Staged) {
		p := f
		if !filepath.IsAbs(p) {
			p = filepath.Join(opts.RootDir, f)
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			existing = append(existing, f)
		}
	}
	if len(existing) == 0 {
		return nil
	}
	cmd := exec.Command("git", append([]string{"add", "--"}, existing...)...)
	if opts.RootDir != "" {
		cmd.Dir = opts.RootDir
	}
	return cmd.Run()
}

// cacheKey computes the cache key for a step, or reports it uncacheable.
// Patterns come from CacheInputs, else OnlyIfChanged. Steps with neither
// are uncacheable; the caller warns about cache:true without inputs.
func cacheKey(s config.Step, opts Options) (string, bool) {
	if !s.Cache {
		return "", false
	}
	patterns := s.CacheInputs
	if len(patterns) == 0 {
		patterns = s.OnlyIfChanged
	}
	if len(patterns) == 0 {
		return "", false
	}
	files, ok := inputHashes(opts.RootDir, patterns)
	if !ok {
		return "", false
	}
	return cache.Key(s.Run, s.Workdir, s.Env, files), true
}

// inputHashes maps each tracked file matching the patterns to its content
// hash. Unreadable files or git failures make the step uncacheable.
func inputHashes(root string, patterns []string) (map[string]string, bool) {
	cmd := exec.Command("git", "ls-files")
	if root != "" {
		cmd.Dir = root
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	files := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.TrimSpace(line)
		if f == "" || !matchAny(patterns, []string{f}) {
			continue
		}
		p := f
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, f)
		}
		h, err := cache.HashFile(p)
		if err != nil {
			return nil, false
		}
		files[filepath.ToSlash(f)] = h
	}
	return files, true
}

// stagedList returns the step's file list: staged files filtered by
// OnlyIfChanged when set, else all staged files.
func stagedList(s config.Step, staged []string) []string {
	var kept []string
	for _, f := range staged {
		if len(s.OnlyIfChanged) == 0 || matchAny(s.OnlyIfChanged, []string{f}) {
			kept = append(kept, f)
		}
	}
	return kept
}

// expandPlaceholders replaces {staged_files} with the shell-quoted file list:
// staged files filtered by OnlyIfChanged when set, else all staged files.
func expandPlaceholders(cmd string, s config.Step, staged []string) string {
	if !strings.Contains(cmd, "{staged_files}") {
		return cmd
	}
	var kept []string
	for _, f := range stagedList(s, staged) {
		kept = append(kept, shellQuote(f))
	}
	return strings.ReplaceAll(cmd, "{staged_files}", strings.Join(kept, " "))
}

// shellQuote wraps s in single quotes for sh.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// matchAny reports whether any glob matches any file. Each pattern is tried
// against the full repo-relative path and against the basename, so "*.go"
// matches "internal/foo/bar.go".
func matchAny(patterns, files []string) bool {
	for _, p := range patterns {
		for _, f := range files {
			f = filepath.ToSlash(f)
			if ok, _ := path.Match(p, f); ok {
				return true
			}
			if ok, _ := path.Match(p, path.Base(f)); ok {
				return true
			}
		}
	}
	return false
}

// StagedFiles lists repo-relative staged paths (added/copied/modified/renamed).
func StagedFiles(dir string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--cached", "--name-only", "--diff-filter=ACMR")
	if dir != "" {
		cmd.Dir = dir
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	var files []string
	for _, l := range strings.Split(buf.String(), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			files = append(files, l)
		}
	}
	return files, nil
}

// RepoRoot returns the enclosing git top-level directory, or "" if none.
func RepoRoot(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// lineStream is an io.Writer that emits complete lines to a callback while
// retaining the last keep lines.
type lineStream struct {
	emit func(string)
	keep int
	buf  []byte
	all  []string
}

func (l *lineStream) Write(p []byte) (int, error) {
	l.buf = append(l.buf, p...)
	for {
		i := bytes.IndexByte(l.buf, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(l.buf[:i]), "\r")
		l.buf = l.buf[i+1:]
		l.all = append(l.all, line)
		if l.emit != nil {
			l.emit(line)
		}
	}
	return len(p), nil
}

func (l *lineStream) tail() []string {
	if len(l.all) == 0 && len(l.buf) > 0 {
		l.all = append(l.all, string(l.buf))
	}
	if len(l.all) <= l.keep {
		return l.all
	}
	return l.all[len(l.all)-l.keep:]
}

// Ensure io.Writer compliance at compile time.
var _ io.Writer = (*lineStream)(nil)
