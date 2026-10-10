# pretty-hooks

Pretty, configurable git hooks in a single binary. A generic step-runner:
each project declares its steps in `.pretty-hooks.yml`, and `pretty-hooks`
executes them with styled live output and a final summary board. Works for
Go, Node, or anything with shell commands.

## Install

```sh
go install github.com/RndmCodeGuy20/pretty-hooks@latest
# optional shorthand:
alias ph=pretty-hooks
```

Prebuilt `pretty-hooks` + `ph` binaries are attached to releases.

## Quickstart

```sh
cd your-repo
pretty-hooks init --preset go    # or: node, minimal
git commit / git push            # hooks run automatically
```

`init` writes `.pretty-hooks.yml` (yours to edit afterwards) and installs
`pre-commit` / `pre-push` shims into `.git/hooks`. Re-run with `--force`
to overwrite.

## Config reference (`.pretty-hooks.yml`)

```yaml
hooks:
  pre-commit:
    steps:
      - name: gofmt              # unique per hook, used by --skip
        run: gofmt -l .          # run via 'sh -c'
        workdir: sub/dir         # optional, relative to repo root
        env: { FOO: bar }        # optional, values allow $VAR expansion
        only_if_changed: ["*.go"] # optional globs vs staged files (pre-commit only)
  pre-push:
    steps:
      - { name: test, run: go test -count=1 ./... }
```

- Steps run in order, **fail-fast** (same as `make`, `task`, CI job steps).
- `only_if_changed` patterns match the full repo-relative path or the
  basename, so `*.go` matches `internal/foo/bar.go`. Ignored by `pre-push`.
- Output is styled on a TTY and plain text otherwise (`NO_COLOR` respected).

## CLI

```sh
pretty-hooks run pre-push [--config PATH] [--skip NAME]...  # flags before or after
HOOKS_OFF=1 git push     # emergency kill-switch, skips execution
git push --no-verify     # git-native bypass (always works)
```

## Non-goals (v1)

Test-framework output parsing, parallel steps, condition expressions,
advisory/non-blocking steps. `gotestsum --format testname` covers Go repos
that want prettier test lines — the runner just displays them.
