package config

// GoPresetYAML is a commented starter config for Go repositories.
// init writes it verbatim; the repo owns it afterwards.
const GoPresetYAML = `# pretty-hooks config - scaffolded by 'pretty-hooks init --preset go'.
# Docs: https://github.com/RndmCodeGuy20/pretty-hooks
#
# Step keys:
#   name:            shown in output (must be unique per hook)
#   run:             shell command (runs via 'sh -c')
#   fix:             optional repair command, runs once on failure with --fix
#   workdir:         optional, relative to repo root (default: repo root)
#   env:             optional extra environment variables
#   only_if_changed: optional globs matched against staged files (pre-commit only)
#
# {staged_files} expands to the quoted staged file list in run and fix.

hooks:
  pre-commit:
    steps:
      - name: gofmt
        run: test -z "$(gofmt -l .)" || (echo "unformatted files:"; gofmt -l .; exit 1)
        fix: gofmt -w {staged_files}
        only_if_changed: ["*.go"]
      - name: vet
        run: go vet ./...
        only_if_changed: ["*.go"]

  pre-push:
    steps:
      - name: build
        run: go build ./...
      - name: vet
        run: go vet ./...
      - name: lint
        run: command -v golangci-lint >/dev/null && golangci-lint run ./... || echo "golangci-lint not installed, skipping"
      - name: test
        run: go test -count=1 ./...
`

// NodePresetYAML is a commented starter config for Node.js repositories.
const NodePresetYAML = `# pretty-hooks config - scaffolded by 'pretty-hooks init --preset node'.
# Docs: https://github.com/RndmCodeGuy20/pretty-hooks

hooks:
  pre-commit:
    steps:
      - name: prettier
        run: npx --no-install prettier --check .
        only_if_changed: ["*.js", "*.ts", "*.jsx", "*.tsx", "*.css", "*.json", "*.md"]
      - name: lint
        run: npm run lint --if-present
        only_if_changed: ["*.js", "*.ts", "*.jsx", "*.tsx"]

  pre-push:
    steps:
      - name: test
        run: npm test -- --passWithNoTests
`

// HygienePresetYAML guards commit hygiene: message format, conflict
// markers, file sizes, and secrets. Thresholds live here; edit freely.
const HygienePresetYAML = `# pretty-hooks config - scaffolded by 'pretty-hooks init --preset hygiene'.
# Hook arguments reach steps as PH_ARG_0..N (commit-msg receives the file).

hooks:
  commit-msg:
    steps:
      - name: conventional-commit
        run: |
          head -n1 "$PH_ARG_0" | grep -Eq '^((feat|fix|docs|chore|refactor|test|ci|perf|revert)(\(.*\))?(!)?: .+|Merge |Revert ")' || (echo "commit message must look like: type(scope): subject"; exit 1)

  pre-commit:
    steps:
      - name: conflict-markers
        run: "hit=$(git grep --cached -l '^<<<<<<< ' 2>/dev/null; git grep --cached -l '^>>>>>>> ' 2>/dev/null); test -z \"$hit\" || (echo \"conflict markers staged:\"; echo \"$hit\"; exit 1)"
      - name: large-files
        run: "hit=$(git diff --cached --name-only -z | tr '\\0' '\\n' | while IFS= read -r f; do test -n \"$f\" || continue; s=$(git cat-file -s \":$f\" 2>/dev/null || echo 0); test \"$s\" -gt 5242880 && echo \"$f\"; done); test -z \"$hit\" || (echo \"files over 5MB staged:\"; echo \"$hit\"; exit 1)"
      - name: secrets
        run: command -v gitleaks >/dev/null && gitleaks protect --staged --verbose || echo "gitleaks not installed, skipping"

  pre-push:
    steps:
      - name: test
        run: echo "add project test command here"
`

// MinimalPresetYAML is an empty skeleton for hand-written configs.
const MinimalPresetYAML = `# pretty-hooks config.
# Docs: https://github.com/RndmCodeGuy20/pretty-hooks

hooks:
  pre-commit:
    steps:
      - name: example
        run: echo hello
        only_if_changed: ["*.go"]

  pre-push:
    steps:
      - name: example
        run: echo hello
`

// Presets maps preset names to scaffold content.
var Presets = map[string]string{
	"go":      GoPresetYAML,
	"node":    NodePresetYAML,
	"hygiene": HygienePresetYAML,
	"minimal": MinimalPresetYAML,
}
