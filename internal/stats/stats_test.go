package stats

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAggregateMath(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	Append(root, "pre-push", "vet", 100*time.Millisecond, "pass")
	Append(root, "pre-push", "vet", 300*time.Millisecond, "pass")
	Append(root, "pre-push", "test", 200*time.Millisecond, "fail")
	Append(root, "pre-commit", "vet", 999*time.Millisecond, "pass")

	aggs := Aggregate(root, "pre-push")
	if len(aggs) != 2 {
		t.Fatalf("got %d groups: %+v", len(aggs), aggs)
	}
	// Sorted by average descending: vet (200) before test (200)? Tie:
	// vet avg = (100+300)/2 = 200, test avg = 200. Order between ties
	// follows first-seen; assert values, not tie order.
	byName := map[string]Agg{}
	for _, a := range aggs {
		byName[a.Step] = a
	}
	vet := byName["vet"]
	if vet.Runs != 2 || vet.AvgMs != 200 || vet.MaxMs != 300 || vet.Fails != 0 {
		t.Errorf("bad vet aggregate: %+v", vet)
	}
	test := byName["test"]
	if test.Runs != 1 || test.AvgMs != 200 || test.Fails != 1 {
		t.Errorf("bad test aggregate: %+v", test)
	}
}

func TestAggregateSkipsCorruptLines(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, ".git", "ph-stats.jsonl")
	content := "not json\n{\"hook\":\"h\",\"step\":\"s\",\"ms\":10,\"status\":\"pass\"}\n\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	aggs := Aggregate(root, "")
	if len(aggs) != 1 || aggs[0].Runs != 1 {
		t.Fatalf("got %+v", aggs)
	}
}

func TestFormatEmpty(t *testing.T) {
	if got := Format(nil); got != "no history yet" {
		t.Errorf("got %q", got)
	}
}

func TestTrimKeepsFileBounded(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxLines+100; i++ {
		Append(root, "h", "s", time.Millisecond, "pass")
	}
	data, err := os.ReadFile(filepath.Join(root, ".git", "ph-stats.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), "\n"); n > maxLines {
		t.Errorf("history grew past bound: %d lines", n)
	}
}
