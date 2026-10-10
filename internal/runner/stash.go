package runner

import (
	"fmt"
	"os/exec"
	"strings"
)

const stashMessage = "pretty-hooks pre-commit"

// stashPushIfDirty stashes unstaged changes to tracked files so steps test
// staged content only. It reports whether it stashed anything.
func stashPushIfDirty(root string) (bool, error) {
	dirty, err := hasUnstagedChanges(root)
	if err != nil {
		return false, err
	}
	if !dirty {
		return false, nil
	}
	cmd := exec.Command("git", "stash", "push", "--keep-index", "-m", stashMessage)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("stash push: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return true, nil
}

// hasUnstagedChanges reports unstaged modifications to tracked files.
func hasUnstagedChanges(root string) (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain=v1", "--untracked-files=no")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		// Second column holds the worktree state; blank means clean.
		if len(line) >= 2 && line[1] != ' ' {
			return true, nil
		}
	}
	return false, nil
}

// stashPop restores the entry created by stashPushIfDirty. Plain 'pop' merges
// and conflicts whenever unstaged edits overlap staged hunks, which
// --keep-index all but guarantees. So restore the recorded worktree directly
// and drop the entry. On any failure the stash entry is kept; the error
// tells the user how to recover.
func stashPop(root string) error {
	run := func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	if out, err := run("checkout", "stash@{0}", "--", "."); err != nil {
		return fmt.Errorf("stash restore failed: %v: %s (work is safe in the stash; recover with: git stash list, then git checkout stash@{0} -- . && git stash drop)", err, out)
	}
	if out, err := run("stash", "drop"); err != nil {
		return fmt.Errorf("stash drop failed: %v: %s (restore succeeded; remove the leftover with: git stash drop)", err, out)
	}
	return nil
}
