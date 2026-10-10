// Package render owns all terminal output: Lip Gloss styling when attached
// to a TTY, guaranteed plain text otherwise (pipes, CI, NO_COLOR, dumb
// terminals) via a forced ASCII color profile.
package render

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

// Status is the outcome of one step.
type Status int

const (
	Pass Status = iota
	Fail
	Skipped
	Cached
)

// Result is the recorded outcome of one step.
type Result struct {
	Name     string
	Status   Status
	Duration time.Duration
	Reason   string   // skip reason, or exit description on failure
	Tail     []string // last output lines, populated on failure
}

// Renderer writes styled (or plain) progress output.
// It is safe for concurrent use; each line prints atomically.
type Renderer struct {
	mu    sync.Mutex
	out   io.Writer
	lr    *lipgloss.Renderer
	color bool
	ci    bool
	buf   []string

	header lipgloss.Style
	step   lipgloss.Style
	dim    lipgloss.Style
	ok     lipgloss.Style
	fail   lipgloss.Style
	skip   lipgloss.Style
	bold   lipgloss.Style
}

// New builds a Renderer. Color is enabled only when out is a terminal,
// NO_COLOR is unset, and TERM is not "dumb". Otherwise styles render as
// plain text (ASCII profile), so piped output stays clean.
func New(out io.Writer) *Renderer {
	color := false
	if f, ok := out.(*os.File); ok {
		_, noColor := os.LookupEnv("NO_COLOR")
		color = term.IsTerminal(int(f.Fd())) && !noColor && os.Getenv("TERM") != "dumb"
	}
	var lr *lipgloss.Renderer
	if color {
		lr = lipgloss.NewRenderer(out)
	} else {
		lr = lipgloss.NewRenderer(out)
		lr.SetColorProfile(termenv.Ascii)
	}
	r := &Renderer{out: out, lr: lr, color: color}
	r.restyle()
	return r
}

// restyle rebuilds every style from the current renderer profile.
func (r *Renderer) restyle() {
	r.header = r.lr.NewStyle().Bold(true)
	r.step = r.lr.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	r.dim = r.lr.NewStyle().Foreground(lipgloss.Color("8"))
	r.ok = r.lr.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	r.fail = r.lr.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	r.skip = r.lr.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	r.bold = r.lr.NewStyle().Bold(true)
}

// EnableCI switches to CI log mode: plain output, collapsible groups,
// and file annotations for failing steps.
func (r *Renderer) EnableCI() {
	r.ci = true
	r.color = false
	r.lr.SetColorProfile(termenv.Ascii)
	r.restyle()
}

// Colored reports whether styled output is active (useful for tests).
func (r *Renderer) Colored() bool { return r.color }

func (r *Renderer) println(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintln(r.out, s)
}

// ciBufferCap bounds the per-step scan buffer for annotations.
const ciBufferCap = 200

// fileLine matches "path/file.ext:LINE: message" for annotation emission.
var fileLine = regexp.MustCompile(`^([^:\s]+\.(go|js|ts|tsx|jsx|py|rb|java|tf)):(\d+):?\d*:?\s*(.*)$`)

// Header opens a hook run.
func (r *Renderer) Header(tool, hook string, n int) {
	r.println(r.header.Render(fmt.Sprintf("%s · %s (%d steps)", tool, hook, n)))
}

// StepBegin announces a step.
func (r *Renderer) StepBegin(i, total int, name, cmd string) {
	if r.ci {
		r.buf = nil
		r.println(fmt.Sprintf("::group::%s - %s", name, cmd))
		return
	}
	r.println(r.step.Render(fmt.Sprintf("› [%d/%d] %s", i, total, name)) + r.dim.Render(fmt.Sprintf("  $ %s", cmd)))
}

// StepLine streams one captured output line, dimmed and indented.
func (r *Renderer) StepLine(line string) {
	r.println(r.dim.Render("    " + line))
	if r.ci && len(r.buf) < ciBufferCap {
		r.buf = append(r.buf, line)
	}
}

// StepOK closes a passing step.
func (r *Renderer) StepOK(name string, d time.Duration) {
	if r.ci {
		r.buf = nil
		r.println("::endgroup::")
	}
	r.println(r.ok.Render(fmt.Sprintf("✓ %s (%s)", name, d.Round(100*time.Millisecond))))
}

// StepFail closes a failing step.
func (r *Renderer) StepFail(name, reason string, d time.Duration) {
	if r.ci {
		for _, line := range r.buf {
			if m := fileLine.FindStringSubmatch(line); m != nil {
				r.println(fmt.Sprintf("::error file=%s,line=%s::%s", m[1], m[3], m[4]))
			}
		}
		r.buf = nil
		r.println("::endgroup::")
	}
	r.println(r.fail.Render(fmt.Sprintf("✗ %s - %s (%s)", name, reason, d.Round(100*time.Millisecond))))
}

// StepSkipped notes a skipped step. Skips never open a group, so there is
// nothing to close in CI mode.
func (r *Renderer) StepSkipped(name, reason string) {
	r.println(r.skip.Render(fmt.Sprintf("⊘ %s - skipped (%s)", name, reason)))
}

// StepCached notes a cache hit.
func (r *Renderer) StepCached(name string) {
	r.println(r.ok.Render(fmt.Sprintf("✓ %s - cached (inputs unchanged)", name)))
}

// Summary prints the final board, including failure tails.
func (r *Renderer) Summary(results []Result) {
	pass, fail, skip, cached := 0, 0, 0, 0
	for _, res := range results {
		switch res.Status {
		case Pass:
			pass++
		case Fail:
			fail++
		case Skipped:
			skip++
		case Cached:
			cached++
		}
	}
	r.println("")
	line := fmt.Sprintf("done: %d passed, %d failed, %d cached, %d skipped", pass, fail, cached, skip)
	if fail > 0 {
		r.println(r.fail.Render(line))
		for _, res := range results {
			if res.Status != Fail || len(res.Tail) == 0 {
				continue
			}
			r.println(r.fail.Render(fmt.Sprintf("--- %s (last output) ---", res.Name)))
			for _, l := range res.Tail {
				r.println(r.dim.Render("    " + l))
			}
		}
	} else {
		r.println(r.bold.Render(line))
	}
}

// Info/Warn/Error are plain helpers for non-step messages.
func (r *Renderer) Info(msg string)  { r.println(msg) }
func (r *Renderer) Warn(msg string)  { r.println(r.skip.Render("warning: " + msg)) }
func (r *Renderer) Error(msg string) { r.println(r.fail.Render("error: " + msg)) }
