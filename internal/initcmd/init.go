// Package initcmd implements 'pretty-hooks init': scaffold the YAML config
// and install git hook shims.
package initcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RndmCodeGuy20/pretty-hooks/internal/config"
	"github.com/RndmCodeGuy20/pretty-hooks/internal/render"
	"github.com/RndmCodeGuy20/pretty-hooks/internal/runner"
)

// managedHooks are the hooks init installs shims for.
var managedHooks = []string{"pre-commit", "pre-push"}

const shimMarker = "Managed by pretty-hooks"

func shim(hook string) string {
	return `#!/bin/sh
# ` + shimMarker + ` (https://github.com/RndmCodeGuy20/pretty-hooks).
# Re-run 'pretty-hooks init --force' to restore. Bypass: '--no-verify' or HOOKS_OFF=1.
command -v pretty-hooks >/dev/null 2>&1 || { echo "pretty-hooks not found on PATH (install: go install github.com/RndmCodeGuy20/pretty-hooks@latest; bypass: --no-verify)"; exit 1; }
exec pretty-hooks run ` + hook + `
`
}

// Execute scaffolds config + shims under the enclosing git repo.
// force overwrites an existing config and foreign hook files.
func Execute(workDir, preset string, force bool, r *render.Renderer) error {
	body, ok := config.Presets[preset]
	if !ok {
		names := make([]string, 0, len(config.Presets))
		for k := range config.Presets {
			names = append(names, k)
		}
		return fmt.Errorf("unknown preset %q (known: %v)", preset, names)
	}
	root := runner.RepoRoot(workDir)
	if root == "" {
		return fmt.Errorf("not inside a git repository")
	}

	cfgPath := filepath.Join(root, config.PrimaryConfigName)
	if _, err := os.Stat(cfgPath); err == nil && !force {
		return fmt.Errorf("%s already exists (use --force to overwrite)", cfgPath)
	}
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		return err
	}
	r.Info(fmt.Sprintf("wrote %s (preset %q)", cfgPath, preset))

	hooksDir := filepath.Join(root, ".git", "hooks")
	for _, hook := range managedHooks {
		p := filepath.Join(hooksDir, hook)
		if data, err := os.ReadFile(p); err == nil && !force {
			if !containsMarker(string(data)) {
				r.Warn(fmt.Sprintf("%s exists and is not ours, leaving it (use --force to replace)", p))
				continue
			}
		}
		// Remove first: p may be a symlink, and writing through it
		// would clobber the link target instead of replacing the hook.
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(p, []byte(shim(hook)), 0o755); err != nil {
			return err
		}
		r.Info(fmt.Sprintf("installed hook %s", p))
	}

	r.Info("")
	r.Info("tip: add a shorthand with:  alias ph=pretty-hooks")
	r.Info("bypass hooks with:  git <command> --no-verify  (or HOOKS_OFF=1)")
	return nil
}

func containsMarker(s string) bool {
	return strings.Contains(s, shimMarker)
}
