# Testing Conventions

Use this file only when the task explicitly points to repository-specific test rules.

## Running Tests

- Use `task test` for the full suite and `task check` for CI-equivalent verification.
- Prefer `task` over raw `go test` so `GOMODCACHE`, `GOCACHE`, and `GOLANGCI_LINT_CACHE` stay inside `.cache/`.
- Use `go test -run TestName ./path/to/pkg` only for focused runs. If you bypass `task`, preserve the same cache environment.

## Suite Requirements

- The full suite must pass with `-race -shuffle=on -count=10`.
- Tests must be race-free, order-independent, and stable across repeats.
- Call `t.Parallel()` in tests and subtests by default. Skip it only when a shared side effect cannot be isolated.
- Never call `t.Fatal` or `t.FailNow` from a helper goroutine.

## Isolation

- Prefer test-scoped helpers: `t.TempDir()`, `t.Setenv()`, `t.Context()` (Go 1.24+), and `t.Cleanup()`.
- Prefer real resources at package boundaries (`httptest`, temp dirs, subprocesses) over deep mocks.
- Mock only external systems that cannot be run locally.

## Linter Exceptions

- `_test.go` files relax `exhaustruct`, `funlen`, `gocognit`, `noctx`, and `contextcheck`.
