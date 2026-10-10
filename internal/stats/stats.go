// Package stats records per-step timing history and reports aggregates.
// History lives in .git/ph-stats.jsonl and never gets committed.
package stats

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// maxLines bounds the history file; writes trim to keepLines.
const (
	maxLines  = 2500
	keepLines = 2000
)

// Record is one step execution.
type Record struct {
	Hook   string `json:"hook"`
	Step   string `json:"step"`
	Ms     int64  `json:"ms"`
	Status string `json:"status"`
	At     string `json:"at"`
}

// Agg is the aggregate for one step.
type Agg struct {
	Step  string
	Runs  int
	AvgMs int64
	MaxMs int64
	Fails int
}

func path(root string) string {
	return filepath.Join(root, ".git", "ph-stats.jsonl")
}

// Append records one execution. It trims the file when it grows past
// maxLines. Errors never propagate; stats must not fail a run.
func Append(root, hook, step string, dur time.Duration, status string) {
	p := path(root)
	lines := readLines(p)
	rec, _ := json.Marshal(Record{
		Hook:   hook,
		Step:   step,
		Ms:     dur.Milliseconds(),
		Status: status,
		At:     time.Now().UTC().Format(time.RFC3339),
	})
	lines = append(lines, string(rec))
	if len(lines) > maxLines {
		lines = lines[len(lines)-keepLines:]
	}
	_ = os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func readLines(p string) []string {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var lines []string
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// Aggregate summarizes history, optionally filtered to one hook.
// Corrupt lines are skipped.
func Aggregate(root, hook string) []Agg {
	byStep := map[string]*Agg{}
	order := []string{}
	for _, line := range readLines(path(root)) {
		var r Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			continue
		}
		if hook != "" && r.Hook != hook {
			continue
		}
		a, ok := byStep[r.Step]
		if !ok {
			a = &Agg{Step: r.Step}
			byStep[r.Step] = a
			order = append(order, r.Step)
		}
		a.Runs++
		a.AvgMs += r.Ms
		if r.Ms > a.MaxMs {
			a.MaxMs = r.Ms
		}
		if r.Status == "fail" {
			a.Fails++
		}
	}
	aggs := make([]Agg, 0, len(byStep))
	for _, name := range order {
		a := byStep[name]
		if a.Runs > 0 {
			a.AvgMs /= int64(a.Runs)
		}
		aggs = append(aggs, *a)
	}
	sort.Slice(aggs, func(i, j int) bool { return aggs[i].AvgMs > aggs[j].AvgMs })
	return aggs
}

// Format renders aggregates as an aligned plain-text table.
func Format(aggs []Agg) string {
	if len(aggs) == 0 {
		return "no history yet"
	}
	var sb strings.Builder
	fmt.Fprintln(&sb, "STEP  RUNS  AVG  MAX  FAILS")
	for _, a := range aggs {
		fmt.Fprintf(&sb, "%s  %d  %s  %s  %d\n",
			a.Step, a.Runs, time.Duration(a.AvgMs)*time.Millisecond,
			time.Duration(a.MaxMs)*time.Millisecond, a.Fails)
	}
	return strings.TrimRight(sb.String(), "\n")
}
