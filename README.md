# pretty-hooks

pretty-hooks runs project checks before `git commit` and `git push`.
It shows the result of each step.

## Installation

Install the tool:

```sh
go install github.com/RndmCodeGuy20/pretty-hooks@latest
```

Optional: add a short name:

```sh
alias ph=pretty-hooks
```

## First Use

Go to the root of the project. Run this command:

```sh
pretty-hooks init --preset go
```

This command creates `.pretty-hooks.yml` and installs the git hooks.
Use `--preset node` for Node.js projects.
Use `--preset minimal` for an empty file.

## Configuration

Edit `.pretty-hooks.yml` to change the steps. Example:

```yaml
hooks:
  pre-push:
    steps:
      - name: build
        run: go build ./...
      - name: test
        run: go test -count=1 ./...
```

Rules:

- Steps run in sequence. The run stops at the first failure.
- `only_if_changed` limits a step to commits that change matching files.
  It applies to `pre-commit` only.
- `{staged_files}` expands to the quoted staged file list.
- Output uses color on a terminal. Output is plain text in pipes and CI systems.

## Repair Failures

A step can declare a `fix` command:

```yaml
- name: gofmt
  run: test -z "$(gofmt -l .)" || exit 1
  fix: gofmt -w {staged_files}
  only_if_changed: ["*.go"]
```

Run with `--fix` (or `PH_FIX=1`). On failure, the tool runs the fix
command, re-stages the repaired files, and runs the step one more time.
The tool never unstages files.

## Commands

- `pretty-hooks run pre-push` runs the steps for a hook.
- `pretty-hooks run pre-push --skip test` skips a step.
- `HOOKS_OFF=1` skips all hooks. Use it only when the tool blocks urgent work.
- `git push --no-verify` bypasses the hooks.

## License

MIT. Refer to LICENSE.
