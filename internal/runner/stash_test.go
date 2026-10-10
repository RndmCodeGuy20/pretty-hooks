package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RndmCodeGuy20/pretty-hooks/internal/config"
	"github.com/RndmCodeGuy20/pretty-hooks/internal/render"
)

func scratchRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	cmds := [][]string{
		{"init", "-q", root},
		{"-C", root, "config", "user.email", "t@t"},
		{"-C", root, "config", "user.name", "t"},
	}
	for _, c := range cmds {
		if out, err := exec.Command("git", c...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", c, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", root, "add", "f.txt").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v %s", err, out)
	}
	if out, err := exec.Command("git", "-C", root, "commit", "-qm", "base").CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v %s", err, out)
	}
	return root
}

func worktree(t *testing.T, root, content string) string {
	t.Helper()
	p := filepath.Join(root, "f.txt")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestStashHidesUnstagedChange(t *testing.T) {
	root := scratchRepo(t)
	worktree(t, root, "staged\n")
	if out, err := exec.Command("git", "-C", root, "add", "f.txt").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v %s", err, out)
	}
	worktree(t, root, "staged plus unstaged\n")

	did, err := stashPushIfDirty(root)
	if err != nil {
		t.Fatal(err)
	}
	if !did {
		t.Fatal("expected stash to trigger on dirty tree")
	}
	if data, _ := os.ReadFile(filepath.Join(root, "f.txt")); string(data) != "staged\n" {
		t.Errorf("worktree should show staged content only, got %q", data)
	}
	if err := stashPop(root); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "f.txt")); string(data) != "staged plus unstaged\n" {
		t.Errorf("unstaged change lost, got %q", data)
	}
}

func TestStashSkipsCleanTree(t *testing.T) {
	root := scratchRepo(t)
	did, err := stashPushIfDirty(root)
	if err != nil {
		t.Fatal(err)
	}
	if did {
		t.Error("clean tree must not stash")
	}
}

func TestStashRestoresOnStepFailure(t *testing.T) {
	root := scratchRepo(t)
	worktree(t, root, "staged\n")
	if out, err := exec.Command("git", "-C", root, "add", "f.txt").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v %s", err, out)
	}
	worktree(t, root, "staged plus unstaged\n")

	var sb strings.Builder
	r := render.New(&sb)
	steps := []config.Step{{Name: "boom", Run: "exit 1"}}
	if _, err := Run(steps, r, &sb, Options{RootDir: root, Stash: true}); err == nil {
		t.Fatal("expected step failure")
	}
	if data, _ := os.ReadFile(filepath.Join(root, "f.txt")); string(data) != "staged plus unstaged\n" {
		t.Errorf("unstaged change lost after failure, got %q", data)
	}
}

func TestStashPopFailureSurfaces(t *testing.T) {
	root := scratchRepo(t)
	worktree(t, root, "change\n")
	if _, err := stashPushIfDirty(root); err != nil {
		t.Fatal(err)
	}
	// Nothing left to pop after a successful pop: second pop must report it.
	if err := stashPop(root); err != nil {
		t.Fatal(err)
	}
	if err := stashPop(root); err == nil {
		t.Error("expected error when no stash entry exists")
	} else if !strings.Contains(err.Error(), "stash") {
		t.Errorf("error should mention recovery, got: %v", err)
	}
}
