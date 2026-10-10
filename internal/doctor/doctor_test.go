package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

const healthyCfg = `hooks:
  pre-commit:
    steps:
      - { name: hi, run: echo hello }
  pre-push:
    steps:
      - { name: list, run: echo world }
`

func fixture(t *testing.T, cfg string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(root, ".pretty-hooks.yml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func installShim(t *testing.T, root, hook string) {
	t.Helper()
	shim := "#!/bin/sh\nexec pretty-hooks run " + hook + "\n"
	if err := os.WriteFile(filepath.Join(root, ".git", "hooks", hook), []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
}

func levels(results []Result) map[string]Level {
	m := map[string]Level{}
	for _, r := range results {
		m[r.Name] = r.Level
	}
	return m
}

func TestHealthyRepo(t *testing.T) {
	root := fixture(t, healthyCfg)
	installShim(t, root, "pre-commit")
	installShim(t, root, "pre-push")
	results, err := Run(root, filepath.Join(root, ".pretty-hooks.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if Failed(results) {
		t.Errorf("healthy repo must pass: %+v", results)
	}
}

func TestMissingShimFails(t *testing.T) {
	root := fixture(t, healthyCfg)
	installShim(t, root, "pre-commit")
	results, err := Run(root, filepath.Join(root, ".pretty-hooks.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if levels(results)["shims"] != Fail {
		t.Errorf("missing pre-push shim must fail: %+v", results)
	}
}

func TestMissingBinaryFails(t *testing.T) {
	root := fixture(t, "hooks:\n  pre-push:\n    steps:\n      - { name: x, run: definitely-not-a-binary-xyz --check }\n")
	installShim(t, root, "pre-push")
	results, err := Run(root, filepath.Join(root, ".pretty-hooks.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if levels(results)["bin:definitely-not-a-binary-xyz"] != Fail {
		t.Errorf("missing binary must fail: %+v", results)
	}
}

func TestPinSkewFails(t *testing.T) {
	root := fixture(t, "hooks:\n  pre-push:\n    steps:\n      - { name: lint, run: echo golangci-lint }\n")
	installShim(t, root, "pre-push")
	bin := t.TempDir()
	fake := "#!/bin/sh\necho 'golangci-lint has version 2.0.0'\n"
	if err := os.WriteFile(filepath.Join(bin, "golangci-lint"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	wf := "jobs:\n  lint:\n    steps:\n      - uses: golangci/golangci-lint-action@v6\n        with:\n          version: v1.64.6\n"
	wfDir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(wfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wfDir, "ci.yml"), []byte(wf), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := Run(root, filepath.Join(root, ".pretty-hooks.yml"))
	if err != nil {
		t.Fatal(err)
	}
	lv := levels(results)["lint-pin"]
	if lv != Fail {
		t.Errorf("v1 pin vs v2 binary must fail, got %v: %+v", lv, results)
	}
}

func TestPinMatchPasses(t *testing.T) {
	root := fixture(t, "hooks:\n  pre-push:\n    steps:\n      - { name: lint, run: echo golangci-lint }\n")
	installShim(t, root, "pre-push")
	bin := t.TempDir()
	fake := "#!/bin/sh\necho 'golangci-lint has version 2.0.0'\n"
	if err := os.WriteFile(filepath.Join(bin, "golangci-lint"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	wf := "jobs:\n  lint:\n    steps:\n      - uses: golangci/golangci-lint-action@v6\n        with:\n          version: v2.14.0\n"
	wfDir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(wfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wfDir, "ci.yml"), []byte(wf), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := Run(root, filepath.Join(root, ".pretty-hooks.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if levels(results)["lint-pin"] != OK {
		t.Errorf("matching pin must pass: %+v", results)
	}
}
