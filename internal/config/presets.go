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
	"minimal": MinimalPresetYAML,
}
