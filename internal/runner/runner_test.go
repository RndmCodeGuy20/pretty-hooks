package runner

import (
	"strings"
	"testing"

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
