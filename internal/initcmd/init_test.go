package initcmd

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RndmCodeGuy20/pretty-hooks/internal/render"
)

func TestExecuteReplacesSymlinkWithoutTouchingTarget(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}

	// Simulate a hook installed as a symlink to a versioned script.
	target := filepath.Join(root, "precious.sh")
	if err := os.WriteFile(target, []byte("#!/bin/sh\necho precious\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, ".git", "hooks", "pre-commit")
	if err := os.Symlink("../../precious.sh", link); err != nil {
		t.Fatal(err)
	}

	r := render.New(io.Discard)
	if err := Execute(root, "minimal", true, r); err != nil {
		t.Fatal(err)
	}

	if data, _ := os.ReadFile(target); string(data) != "#!/bin/sh\necho precious\n" {
		t.Fatalf("symlink target was clobbered:\n%s", data)
	}
	fi, err := os.Lstat(link)
	if err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("hook path should be a regular file, got %v %v", fi, err)
	}
	data, err := os.ReadFile(link)
	if err != nil || !strings.Contains(string(data), shimMarker) {
		t.Fatalf("hook file missing marker: %v\n%s", err, data)
	}
	if _, err := os.Stat(filepath.Join(root, ".pretty-hooks.yml")); err != nil {
		t.Fatalf("config not scaffolded: %v", err)
	}
}
