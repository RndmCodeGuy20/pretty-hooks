// Package doctor checks that hooks will work before they run:
// config validity, installed shims, required binaries, and CI pin parity.
package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/RndmCodeGuy20/pretty-hooks/internal/config"
	"gopkg.in/yaml.v3"
)

// Level is the severity of one check.
type Level int

const (
	OK Level = iota
	Warn
	Fail
)

// Result is the outcome of one check.
type Result struct {
	Name   string
	Level  Level
	Detail string
}

// shellBuiltins are first tokens that need no binary on PATH.
var shellBuiltins = map[string]bool{
	"sh": true, "echo": true, "cd": true, "test": true,
	"command": true, "git": true, "exit": true,
}

// tokenChars limits first-token checks to plain command names.
// Compound commands (pipes, subshells, quotes) are skipped.
var tokenChars = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

// Run loads the config and executes every check.
func Run(root, cfgPath string) ([]Result, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}
	results := []Result{
		checkShims(root, cfg),
	}
	results = append(results, checkBinaries(cfg)...)
	if r, ok := checkGolangciPin(root, cfg); ok {
		results = append(results, r)
	}
	return results, nil
}

// Failed reports whether any check failed.
func Failed(results []Result) bool {
	for _, r := range results {
		if r.Level == Fail {
			return true
		}
	}
	return false
}

func checkShims(root string, cfg *config.File) Result {
	var missing []string
	for hook := range cfg.Hooks {
		data, err := os.ReadFile(filepath.Join(root, ".git", "hooks", hook))
		if err != nil || !strings.Contains(string(data), "pretty-hooks run "+hook) {
			missing = append(missing, hook)
		}
	}
	if len(missing) > 0 {
		return Result{"shims", Fail, fmt.Sprintf("hooks not installed: %s (run: pretty-hooks init)", strings.Join(missing, ", "))}
	}
	return Result{"shims", OK, "all configured hooks installed"}
}

// checkBinaries verifies the first token of each step resolves on PATH.
func checkBinaries(cfg *config.File) []Result {
	seen := map[string]bool{}
	var results []Result
	for _, h := range cfg.Hooks {
		for _, s := range h.Steps {
			fields := strings.Fields(s.Run)
			if len(fields) == 0 {
				continue
			}
			bin := fields[0]
			if shellBuiltins[bin] || !tokenChars.MatchString(bin) || seen[bin] {
				continue
			}
			seen[bin] = true
			if _, err := exec.LookPath(bin); err != nil {
				results = append(results, Result{"bin:" + bin, Fail, "not found on PATH"})
			} else {
				results = append(results, Result{"bin:" + bin, OK, "found on PATH"})
			}
		}
	}
	return results
}

// checkGolangciPin compares the CI-pinned major version against the local
// binary. It returns false when the check does not apply (no references,
// no workflows directory).
func checkGolangciPin(root string, cfg *config.File) (Result, bool) {
	referenced := false
	for _, h := range cfg.Hooks {
		for _, s := range h.Steps {
			if strings.Contains(s.Run, "golangci-lint") {
				referenced = true
			}
		}
	}
	pins := workflowLintPins(filepath.Join(root, ".github", "workflows"))
	if len(pins) == 0 && !referenced {
		return Result{}, false
	}
	local, err := golangciMajor()
	if err != nil {
		return Result{"lint-pin", Warn, "golangci-lint not installed, cannot compare pins"}, true
	}
	for _, pin := range pins {
		if major(pin) != "" && major(pin) != local {
			return Result{"lint-pin", Fail,
				fmt.Sprintf("CI pins golangci-lint %s but local is v%s (config format differs per major)", pin, local)}, true
		}
	}
	if len(pins) == 0 {
		return Result{"lint-pin", OK, "no CI pin to compare"}, true
	}
	return Result{"lint-pin", OK, fmt.Sprintf("CI pin matches local (v%s)", local)}, true
}

func workflowLintPins(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var pins []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || (!strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml")) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var wf struct {
			Jobs map[string]struct {
				Steps []struct {
					Uses string            `yaml:"uses"`
					With map[string]string `yaml:"with"`
				} `yaml:"steps"`
			} `yaml:"jobs"`
		}
		if err := yaml.Unmarshal(data, &wf); err != nil {
			continue
		}
		for _, job := range wf.Jobs {
			for _, step := range job.Steps {
				if strings.Contains(step.Uses, "golangci/golangci-lint-action") {
					if v, ok := step.With["version"]; ok {
						pins = append(pins, v)
					}
				}
			}
		}
	}
	return pins
}

func golangciMajor() (string, error) {
	out, err := exec.Command("golangci-lint", "version").CombinedOutput()
	if err != nil {
		return "", err
	}
	m := major(string(out))
	if m == "" {
		return "", fmt.Errorf("cannot parse version output")
	}
	return m, nil
}

var majorRe = regexp.MustCompile(`v?(\d+)\.`)

// major extracts the leading major number from "v1.64.6" or "2.12.2".
func major(s string) string {
	m := majorRe.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[1]
}
