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
