package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPresetsParse(t *testing.T) {
	for name, body := range Presets {
		path := filepath.Join(t.TempDir(), ".pretty-hooks.yml")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		f, err := Load(path)
		if err != nil {
			t.Errorf("preset %q failed to load: %v", name, err)
			continue
		}
		for _, hook := range []string{"pre-commit", "pre-push"} {
			if _, err := f.StepsFor(hook); err != nil {
				t.Errorf("preset %q: %v", name, err)
			}
		}
	}
}

func TestValidateRejectsBadSteps(t *testing.T) {
	for _, tc := range []struct {
		name string
		file File
	}{
		{"empty hooks", File{}},
		{"empty steps", File{Hooks: map[string]Hook{"pre-push": {}}}},
		{"nameless step", File{Hooks: map[string]Hook{"pre-push": {Steps: []Step{{Run: "x"}}}}}},
		{"runless step", File{Hooks: map[string]Hook{"pre-push": {Steps: []Step{{Name: "x"}}}}}},
		{"dup names", File{Hooks: map[string]Hook{"pre-push": {Steps: []Step{{Name: "x", Run: "a"}, {Name: "x", Run: "b"}}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.file.Validate(); err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}
}

func TestFindWalksUp(t *testing.T) {
	root := t.TempDir()
	cfg := filepath.Join(root, PrimaryConfigName)
	if err := os.WriteFile(cfg, []byte("hooks:\n  pre-push:\n    steps: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	found, err := Find(nested)
	if err != nil {
		t.Fatal(err)
	}
	if found != cfg {
		t.Errorf("got %q, want %q", found, cfg)
	}
}
