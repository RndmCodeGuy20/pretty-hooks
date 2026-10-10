// Package config loads and validates pretty-hooks YAML configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	// PrimaryConfigName is the canonical config file name.
	PrimaryConfigName = ".pretty-hooks.yml"
	// AltConfigName is accepted as a fallback extension variant.
	AltConfigName = ".pretty-hooks.yaml"
)

// Step is a single command executed by a hook.
type Step struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
	// Fix repairs failure causes (formatters with --write). Runs once on
	// failure when --fix is passed, then the step runs one final time.
	Fix string `yaml:"fix,omitempty"`
	// Workdir is resolved relative to the repository root. Empty means repo root.
	Workdir string `yaml:"workdir,omitempty"`
	// Env adds (or overrides) environment variables for this step.
	// Values go through os.ExpandEnv.
	Env map[string]string `yaml:"env,omitempty"`
	// OnlyIfChanged holds glob patterns matched against staged files.
	// Each pattern matches the full repo-relative path or the basename.
	// Only evaluated for pre-commit; ignored by pre-push (all steps run).
	OnlyIfChanged []string `yaml:"only_if_changed,omitempty"`
}

// Hook is the ordered step list for one git hook.
type Hook struct {
	Steps []Step `yaml:"steps"`
}

// File is the root of the YAML config.
type File struct {
	Hooks map[string]Hook `yaml:"hooks"`
}

// Find walks up from startDir looking for a config file, stopping at the
// filesystem root or the first directory containing a .git entry.
func Find(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}
	for {
		for _, name := range []string{PrimaryConfigName, AltConfigName} {
			p := filepath.Join(dir, name)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p, nil
			}
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return "", fmt.Errorf("no %s found (searched up to repo root %s)", PrimaryConfigName, dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s found", PrimaryConfigName)
		}
		dir = parent
	}
}

// Load reads, parses, and validates a config file.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := f.Validate(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", path, err)
	}
	return &f, nil
}

// Validate rejects empty names, empty commands, and duplicate step names per hook.
func (f *File) Validate() error {
	if len(f.Hooks) == 0 {
		return fmt.Errorf("no hooks defined (expected a \"hooks:\" mapping)")
	}
	for hook, h := range f.Hooks {
		if len(h.Steps) == 0 {
			return fmt.Errorf("hook %q has no steps", hook)
		}
		seen := map[string]bool{}
		for i, s := range h.Steps {
			if s.Name == "" {
				return fmt.Errorf("hook %q step %d has no name", hook, i+1)
			}
			if s.Run == "" {
				return fmt.Errorf("hook %q step %q has no run command", hook, s.Name)
			}
			if seen[s.Name] {
				return fmt.Errorf("hook %q has duplicate step name %q", hook, s.Name)
			}
			seen[s.Name] = true
		}
	}
	return nil
}

// StepsFor returns the steps for a hook, or an error listing known hooks.
func (f *File) StepsFor(hook string) ([]Step, error) {
	h, ok := f.Hooks[hook]
	if !ok {
		known := make([]string, 0, len(f.Hooks))
		for k := range f.Hooks {
			known = append(known, k)
		}
		return nil, fmt.Errorf("no %q hook configured (known: %v)", hook, known)
	}
	return h.Steps, nil
}
