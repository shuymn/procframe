# ADR-004: Project-local tool and build cache

## Status

Accepted

## Context

Go tooling uses several caches (`GOMODCACHE`, `GOCACHE`, `GOLANGCI_LINT_CACHE`). By default these are stored under `$GOPATH` or `$HOME/.cache`, shared across all projects on the machine.

## Decision

Override cache directories to project-local `.cache/` via Taskfile environment variables:

```yaml
env:
  GOMODCACHE: "{{.ROOT_DIR}}/.cache/go-mod"
  GOCACHE: "{{.ROOT_DIR}}/.cache/go-build"
  GOLANGCI_LINT_CACHE: "{{.ROOT_DIR}}/.cache/golangci-lint"
```

Reasons:

- **Agent write permissions**: Coding agents may lack write access outside the repo root or `/tmp`. Project-local caches ensure `go build` and `go test` work in restricted environments.
- **Isolation**: Different projects do not interfere with each other's caches.
- **LLM/agent accessibility**: AI coding agents can inspect module source code within the project tree without navigating global paths.
- **Easy cleanup**: `rm -rf .cache/` resets all caches for the project.

## Consequences

- `.cache/` is gitignored.
- Cache isolation only applies when running via `task`. Direct `go` commands use global caches unless the env vars are set explicitly.
