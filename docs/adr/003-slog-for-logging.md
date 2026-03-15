# ADR-003: slog for structured logging

## Status

Accepted

## Context

Go logging library options:

- **`log`**: Standard library, unstructured. Insufficient for production observability.
- **`log/slog`**: Standard library (Go 1.21+), structured logging with levels.
- **zerolog**: High-performance, zero-allocation. Third-party dependency.
- **zap**: Widely adopted, feature-rich. Third-party dependency.

## Decision

Use `log/slog` from the standard library.

Reasons:

- **Zero dependencies**: Aligns with the stdlib-first policy.
- **Standard interface**: Third-party handlers (JSON, OpenTelemetry) can be plugged in without changing call sites.
- **Context-aware**: `slog.InfoContext(ctx, ...)` propagates trace context naturally.
- **Sufficient for most use cases**: Unless sub-microsecond logging is required, slog's performance is adequate.

## Consequences

- `fmt.Print*` and `panic` are forbidden by linter (`forbidigo`). Use `slog` for output and return errors instead of panicking.
- `context.Background()` / `context.TODO()` are forbidden by linter; pass inherited context from callers.
- The slog handler can be swapped without changing call sites.
