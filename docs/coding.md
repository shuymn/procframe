# Coding Conventions

Use this file only when the task explicitly points to repository-specific implementation rules.

## Runtime Boundaries

- Use `log/slog` for structured logs.
- Do not commit `fmt.Print*` debugging.
- Do not `panic` outside process-boundary code such as `main` or `cmd`.
- Thread inherited `context.Context` through call paths. Do not call `context.Background()` or `context.TODO()` in application code.
- Inject a clock or time source instead of calling `time.Now()` directly.
- Build HTTP requests with context and use an explicit `http.Client`; avoid package-level `http.Get` / `http.Post` helpers.

## Errors and APIs

- Return errors and let the caller decide whether to log them. Do not log and return the same error.
- Use `%w` only when callers should inspect the wrapped error with `errors.Is` or `errors.As`; otherwise wrap with `%v` at the boundary.
- Put `context.Context` first in function signatures and never store it in a struct.
- Prefer synchronous functions. Add concurrency at the caller boundary unless the API is inherently asynchronous.
- Define interfaces in the consuming package and return concrete types from constructors.

## Struct Discipline

- Types named `Config`, `Options`, `Params`, `Query`, or `Event` are treated as configuration or data boundaries.
- In non-test code, initialize those structs explicitly so `exhaustruct` can catch drift.
- See [.golangci.yaml](../.golangci.yaml) for the exact enforced bans and exceptions.
