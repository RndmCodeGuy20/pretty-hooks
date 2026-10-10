// Command pretty-hooks (alias: ph) runs pretty, configurable git hooks.
//
// Usage:
//
//	pretty-hooks init [--preset go|node|minimal] [--force]
//	pretty-hooks run <hook> [--config PATH] [--skip NAME]...
//	pretty-hooks version
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/RndmCodeGuy20/pretty-hooks/internal/config"
	"github.com/RndmCodeGuy20/pretty-hooks/internal/doctor"
	"github.com/RndmCodeGuy20/pretty-hooks/internal/initcmd"
	"github.com/RndmCodeGuy20/pretty-hooks/internal/render"
	"github.com/RndmCodeGuy20/pretty-hooks/internal/runner"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

const toolName = "pretty-hooks"

func main() {
	os.Exit(dispatch(os.Args[1:], os.Stdout))
}

func dispatch(args []string, out *os.File) int {
	if len(args) == 0 {
		usage(out)
		return 2
	}
	switch args[0] {
	case "init":
		return cmdInit(args[1:], out)
	case "run":
		return cmdRun(args[1:], out)
	case "doctor":
		return cmdDoctor(args[1:], out)
	case "version", "--version", "-v":
		fmt.Fprintln(out, toolName, version)
		return 0
	case "help", "--help", "-h":
		usage(out)
		return 0
	default:
		fmt.Fprintf(out, "unknown command %q\n\n", args[0])
		usage(out)
		return 2
	}
}

func usage(out *os.File) {
	fmt.Fprintf(out, `usage: %s <command> [flags]

commands:
  init [--preset go|node|minimal] [--force]   scaffold config + install git shims
  run <hook> [--config PATH] [--skip NAME]... [--fix] [--no-cache]
                                              execute a hook's steps (fail-fast)
  doctor [--config PATH]                      check setup (shims, binaries, CI pins)
  version                                     print version

env:
  HOOKS_OFF=1   skip execution (emergency kill-switch)
  PH_FIX=1      same as --fix (repair, re-stage, re-run once)
  NO_COLOR=1    force plain output
`, toolName)
}

func cmdInit(args []string, out *os.File) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(out)
	preset := fs.String("preset", "go", "scaffold preset: go|node|minimal")
	force := fs.Bool("force", false, "overwrite existing config and foreign hook files")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	r := render.New(out)
	cwd, _ := os.Getwd()
	if err := initcmd.Execute(cwd, *preset, *force, r); err != nil {
		r.Error(err.Error())
		return 1
	}
	return 0
}

// repeatable --skip NAME flag.

func cmdDoctor(args []string, out *os.File) int {
	cfgPath := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--config" && i+1 < len(args):
			i++
			cfgPath = args[i]
		case strings.HasPrefix(a, "--config="):
			cfgPath = strings.TrimPrefix(a, "--config=")
		case a == "--help" || a == "-h":
			fmt.Fprintln(out, "usage: pretty-hooks doctor [--config PATH]")
			return 0
		default:
			fmt.Fprintf(out, "unknown argument %q\n", a)
			return 2
		}
	}
	r := render.New(out)
	if cfgPath == "" {
		cwd, _ := os.Getwd()
		var err error
		cfgPath, err = config.Find(cwd)
		if err != nil {
			r.Error(err.Error())
			return 1
		}
	}
	root := runner.RepoRoot("")
	if root == "" {
		if cwd, err := os.Getwd(); err == nil {
			root = cwd
		}
	}
	results, err := doctor.Run(root, cfgPath)
	if err != nil {
		r.Error(err.Error())
		return 1
	}
	failed := false
	for _, res := range results {
		var mark string
		switch res.Level {
		case doctor.OK:
			mark = "ok"
		case doctor.Warn:
			mark = "warn"
		default:
			mark = "FAIL"
			failed = true
		}
		r.Info(fmt.Sprintf("%-4s %-16s %s", mark, res.Name, res.Detail))
	}
	if failed {
		return 1
	}
	return 0
}

func cmdRun(args []string, out *os.File) int {
	// Manual parsing (instead of flag.FlagSet) so flags may appear
	// before or after the hook name: 'run pre-push --skip lint'.
	var skips []string
	cfgPath := ""
	fixFlag := false
	noCache := false
	var positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--skip" && i+1 < len(args):
			i++
			skips = append(skips, args[i])
		case strings.HasPrefix(a, "--skip="):
			skips = append(skips, strings.TrimPrefix(a, "--skip="))
		case a == "--config" && i+1 < len(args):
			i++
			cfgPath = args[i]
		case strings.HasPrefix(a, "--config="):
			cfgPath = strings.TrimPrefix(a, "--config=")
		case a == "--fix":
			fixFlag = true
		case a == "--no-cache":
			noCache = true
		case a == "--help" || a == "-h":
			fmt.Fprintln(out, "usage: pretty-hooks run <hook> [--config PATH] [--skip NAME]... [--fix] [--no-cache]")
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(out, "unknown flag %q\n", a)
			return 2
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) != 1 {
		fmt.Fprintln(out, "usage: pretty-hooks run <hook> [--config PATH] [--skip NAME]... [--fix] [--no-cache]")
		return 2
	}
	hook := positional[0]
	r := render.New(out)

	if os.Getenv("HOOKS_OFF") == "1" {
		r.Warn("HOOKS_OFF=1, skipping " + hook)
		return 0
	}

	path := cfgPath
	if path == "" {
		cwd, _ := os.Getwd()
		var err error
		path, err = config.Find(cwd)
		if err != nil {
			r.Error(err.Error())
			return 1
		}
	}
	cfg, err := config.Load(path)
	if err != nil {
		r.Error(err.Error())
		return 1
	}
	steps, err := cfg.StepsFor(hook)
	if err != nil {
		r.Error(err.Error())
		return 1
	}
	stash := hook == "pre-commit" && cfg.Hooks[hook].Stash

	root := runner.RepoRoot("")
	if root == "" {
		if cwd, err := os.Getwd(); err == nil {
			root = cwd
		}
	}

	var staged []string
	filterChanged := false
	if hook == "pre-commit" {
		// Best effort: if git can't tell us, run everything.
		filterChanged = true
		staged, _ = runner.StagedFiles(root)
	}

	skip := map[string]bool{}
	for _, s := range skips {
		skip[s] = true
	}

	r.Header(toolName, hook, len(steps))
	for _, s := range steps {
		if s.Cache && len(s.CacheInputs) == 0 && len(s.OnlyIfChanged) == 0 {
			r.Warn(fmt.Sprintf("step %q sets cache without inputs; add cache_inputs or only_if_changed", s.Name))
		}
	}
	results, runErr := runner.Run(steps, r, out, runner.Options{
		Skip:          skip,
		Staged:        staged,
		FilterChanged: filterChanged,
		Fix:           fixFlag || os.Getenv("PH_FIX") == "1",
		Stash:         stash,
		Hook:          hook,
		NoCache:       noCache,
		RootDir:       root,
	})
	r.Summary(results)
	if runErr != nil {
		r.Error(runErr.Error())
		return 1
	}
	return 0
}
