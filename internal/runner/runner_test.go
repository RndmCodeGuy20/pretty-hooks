package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RndmCodeGuy20/pretty-hooks/internal/config"
	"github.com/RndmCodeGuy20/pretty-hooks/internal/render"
)

func TestMatchAny(t *testing.T) {
	for _, tc := range []struct {
		name     string
		patterns []string
		files    []string
		want     bool
	}{
		{"ext matches nested", []string{"*.go"}, []string{"internal/foo/bar.go"}, true},
		{"ext misses", []string{"*.go"}, []string{"README.md"}, false},
		{"dir prefix", []string{"docs/*"}, []string{"docs/guide.md"}, true},
		{"no files", []string{"*.go"}, nil, false},
		{"multiple patterns", []string{"*.md", "*.go"}, []string{"main.go"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchAny(tc.patterns, tc.files); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRunFailFast(t *testing.T) {
	var sb strings.Builder
	r := render.New(&sb)
	steps := []config.Step{
		{Name: "ok", Run: "echo hi"},
		{Name: "boom", Run: "echo before-fail; exit 3"},
		{Name: "never", Run: "echo unreachable"},
	}
	results, err := Run(steps, r, &sb, Options{RootDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected failure, got nil")
	}
	fe, ok := err.(*FailedError)
	if !ok || fe.Step != "boom" || fe.Code != 3 {
		t.Fatalf("unexpected error value: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("fail-fast violated: got %d results, want 2", len(results))
	}
	if out := sb.String(); strings.Contains(out, "unreachable") {
		t.Error("third step executed despite fail-fast")
	}
}

func TestRunSkipAndFilter(t *testing.T) {
	var sb strings.Builder
	r := render.New(&sb)
	steps := []config.Step{
		{Name: "skipped", Run: "exit 1"},
		{Name: "filtered", Run: "exit 1", OnlyIfChanged: []string{"*.go"}},
		{Name: "runs", Run: "echo yes"},
	}
	results, err := Run(steps, r, &sb, Options{
		RootDir:       t.TempDir(),
		Skip:          map[string]bool{"skipped": true},
		Staged:        []string{"README.md"},
		FilterChanged: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	for _, res := range results {
		if res.Name != "runs" && res.Status != render.Skipped {
			t.Errorf("step %q should be skipped, got %v", res.Name, res.Status)
		}
		if res.Name == "runs" && res.Status != render.Pass {
			t.Errorf("step %q should pass, got %v", res.Name, res.Status)
		}
	}
}

func TestExpandPlaceholders(t *testing.T) {
	s := config.Step{Name: "x", Run: "gofmt -l {staged_files}", OnlyIfChanged: []string{"*.go"}}
	got := expandPlaceholders(s.Run, s, []string{"a.go", "docs/b.md", "o'Brien.go"})
	want := "gofmt -l 'a.go' 'o'\\''Brien.go'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExpandPlaceholdersNoFilter(t *testing.T) {
	s := config.Step{Name: "x", Run: "echo {staged_files}"}
	got := expandPlaceholders(s.Run, s, []string{"a.go", "b.md"})
	if got != "echo 'a.go' 'b.md'" {
		t.Errorf("got %q", got)
	}
}

func TestExpandPlaceholdersAbsent(t *testing.T) {
	s := config.Step{Name: "x", Run: "go vet ./..."}
	if got := expandPlaceholders(s.Run, s, []string{"a.go"}); got != s.Run {
		t.Errorf("got %q, want unchanged", got)
	}
}

func TestFixRepairsAndReruns(t *testing.T) {
	dir := t.TempDir()
	flag := filepath.Join(dir, "dirty.txt")
	if err := os.WriteFile(flag, []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	r := render.New(&sb)
	steps := []config.Step{{
		Name: "fmt",
		Run:  "test ! -f dirty.txt",
		Fix:  "rm dirty.txt",
	}}
	results, err := Run(steps, r, &sb, Options{RootDir: dir, Fix: true})
	if err != nil {
		t.Fatalf("expected recovery, got %v", err)
	}
	if len(results) != 1 || results[0].Status != render.Pass {
		t.Fatalf("got %+v", results)
	}
	if _, statErr := os.Stat(flag); !os.IsNotExist(statErr) {
		t.Error("fix did not remove the file")
	}
}

func TestFixRunsOnlyWithFlag(t *testing.T) {
	dir := t.TempDir()
	flag := filepath.Join(dir, "dirty.txt")
	if err := os.WriteFile(flag, []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	r := render.New(&sb)
	steps := []config.Step{{
		Name: "fmt",
		Run:  "test ! -f dirty.txt",
		Fix:  "rm dirty.txt",
	}}
	if _, err := Run(steps, r, &sb, Options{RootDir: dir}); err == nil {
		t.Fatal("expected failure without Fix option")
	}
	if _, statErr := os.Stat(flag); statErr != nil {
		t.Error("fix ran without the flag")
	}
}

func TestFixSingleRetry(t *testing.T) {
	var sb strings.Builder
	r := render.New(&sb)
	steps := []config.Step{{
		Name: "broken",
		Run:  "exit 1",
		Fix:  "exit 0",
	}}
	results, err := Run(steps, r, &sb, Options{RootDir: t.TempDir(), Fix: true})
	if err == nil {
		t.Fatal("expected failure after fix did not help")
	}
	if len(results) != 1 || results[0].Status != render.Fail {
		t.Fatalf("got %+v", results)
	}
	if n := strings.Count(sb.String(), "[1/1] broken"); n != 2 {
		t.Errorf("step ran %d times, want exactly 2 (try + one retry)", n)
	}
}

func TestGroupRunsConcurrently(t *testing.T) {
	var sb strings.Builder
	r := render.New(&sb)
	steps := []config.Step{
		{Name: "a", Run: "sleep 1", Group: "g"},
		{Name: "b", Run: "sleep 1", Group: "g"},
	}
	start := time.Now()
	results, err := Run(steps, r, &sb, Options{RootDir: t.TempDir()})
	dur := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results", len(results))
	}
	if results[0].Name != "a" || results[1].Name != "b" {
		t.Errorf("result order not preserved: %+v", results)
	}
	if dur >= 1900*time.Millisecond {
		t.Errorf("steps ran sequentially, took %v", dur)
	}
}

func TestGroupCollectsAllFailures(t *testing.T) {
	var sb strings.Builder
	r := render.New(&sb)
	steps := []config.Step{
		{Name: "bad1", Run: "exit 1", Group: "g"},
		{Name: "bad2", Run: "exit 2", Group: "g"},
		{Name: "never", Run: "exit 0"},
	}
	results, err := Run(steps, r, &sb, Options{RootDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected failure")
	}
	if len(results) != 2 {
		t.Fatalf("both group members must run, got %d results", len(results))
	}
	for _, res := range results {
		if res.Status != render.Fail {
			t.Errorf("step %q should fail", res.Name)
		}
	}
}

func TestGroupLabelsOutput(t *testing.T) {
	var sb strings.Builder
	r := render.New(&sb)
	steps := []config.Step{
		{Name: "a", Run: "echo hello", Group: "g"},
		{Name: "solo", Run: "echo plain"},
	}
	if _, err := Run(steps, r, &sb, Options{RootDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.Contains(out, "a | hello") {
		t.Errorf("grouped output missing label:\n%s", out)
	}
	if strings.Contains(out, "solo | plain") {
		t.Errorf("ungrouped output must not be labeled:\n%s", out)
	}
}

func TestOnlyIfChangedEmptyStagedSkips(t *testing.T) {
	var sb strings.Builder
	r := render.New(&sb)
	steps := []config.Step{
		{Name: "go-only", Run: "exit 1", OnlyIfChanged: []string{"*.go"}},
	}
	// Empty staged list + filtering on must skip (regression: nil-vs-empty).
	results, err := Run(steps, r, &sb, Options{RootDir: t.TempDir(), Staged: []string{}, FilterChanged: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Status != render.Skipped {
		t.Fatalf("expected 1 skipped result, got %+v", results)
	}
	// Same setup with filtering off (pre-push) must run.
	sb.Reset()
	results, err = Run(steps, r, &sb, Options{RootDir: t.TempDir(), Staged: []string{}})
	if err == nil {
		t.Fatal("expected failure: step should have run with filtering off")
	}
	if len(results) != 1 || results[0].Status != render.Fail {
		t.Fatalf("expected 1 failed result, got %+v", results)
	}
}
