package render

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPlainFallbackHasNoANSI(t *testing.T) {
	var sb strings.Builder
	r := New(&sb) // not a TTY -> plain
	if r.Colored() {
		t.Error("expected plain output for non-terminal writer")
	}
	r.Header("pretty-hooks", "pre-push", 2)
	r.StepBegin(1, 2, "build", "go build ./...")
	r.StepLine("some output")
	r.StepOK("build", 1500*time.Millisecond)
	r.StepSkipped("lint", "--skip")
	r.StepFail("test", "exit 1", 2500*time.Millisecond)
	r.Summary([]Result{
		{Name: "build", Status: Pass},
		{Name: "lint", Status: Skipped, Reason: "--skip"},
		{Name: "test", Status: Fail, Reason: "exit 1", Tail: []string{"FAIL pkg"}},
	})
	if out := sb.String(); strings.Contains(out, "\x1b[") {
		t.Errorf("plain output contains ANSI escapes:\n%q", out)
	}
}

func TestConcurrentWritesKeepWholeLines(t *testing.T) {
	var sb strings.Builder
	r := New(&sb)
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				r.StepLine(strings.Repeat("x", 40))
			}
		}(w)
	}
	wg.Wait()
	for _, line := range strings.Split(strings.TrimRight(sb.String(), "\n"), "\n") {
		if len(line) != 4+40 {
			t.Fatalf("interleaved line of length %d: %q", len(line), line)
		}
	}
}

func TestCIEmitsGroups(t *testing.T) {
	var sb strings.Builder
	r := New(&sb)
	r.EnableCI()
	r.StepBegin(1, 1, "vet", "go vet ./...")
	r.StepLine("all good")
	r.StepOK("vet", time.Second)
	out := sb.String()
	for _, want := range []string{"::group::vet", "::endgroup::"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("CI output must be plain:\n%q", out)
	}
}

func TestCIAnnotatesFileLinesOnFailure(t *testing.T) {
	var sb strings.Builder
	r := New(&sb)
	r.EnableCI()
	r.StepBegin(1, 1, "vet", "go vet ./...")
	r.StepLine("pkg/a.go:12: something broke")
	r.StepLine("unrelated noise")
	r.StepFail("vet", "exit 1", time.Second)
	out := sb.String()
	if !strings.Contains(out, "::error file=pkg/a.go,line=12::something broke") {
		t.Errorf("missing annotation in:\n%s", out)
	}
	if strings.Contains(out, "::error file=unrelated") {
		t.Errorf("non-matching line was annotated:\n%s", out)
	}
}

func TestCINoAnnotationsOnPass(t *testing.T) {
	var sb strings.Builder
	r := New(&sb)
	r.EnableCI()
	r.StepBegin(1, 1, "vet", "go vet ./...")
	r.StepLine("pkg/a.go:12: note")
	r.StepOK("vet", time.Second)
	if out := sb.String(); strings.Contains(out, "::error") {
		t.Errorf("passing step must not annotate:\n%s", out)
	}
}
