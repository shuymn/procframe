# ADR-005: Config Secret Formatting

## Status

Accepted

## Context

Theme 4 adds generated config loaders from `config.proto` and allows fields to be marked with `secret=true`.

The first implementation exposed a public `Redacted()` helper on generated config types. That works, but it relies on callers remembering to opt into the redacted view before logging. In practice, the generated runtime config pointer returned from `LoadRuntimeConfig` is often logged directly with `fmt`, `slog`, or test failure output, so secret safety needs to hold for that default formatting path.

## Decision

- Generated config types implement `fmt.Formatter`.
- Formatting a generated runtime config pointer masks every field marked with `secret=true`.
- Secret parse failures from env/bootstrap inputs also redact the raw value from returned errors.
- `LoadRuntimeConfig` remains the generated constructor entrypoint; secret-safe formatting is part of the generated runtime config pointer contract, not a separate public `Redacted()` API.

## Consequences

- Callers can log the generated runtime config pointer directly without manually creating a redacted copy first.
- The generated code must keep masking logic aligned with `secret=true` field metadata.
- Tests must verify both merge-chain behavior and pointer-based formatted output redaction.
