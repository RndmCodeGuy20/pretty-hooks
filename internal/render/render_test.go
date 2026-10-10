package render

import (
	"strings"
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
