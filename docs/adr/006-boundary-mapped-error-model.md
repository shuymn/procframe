# ADR-006: Boundary-mapped error model

## Status

Accepted

## Context

The initial runtime exposed a public `procframe.Error` interface with transport-oriented methods such as canonical code and retryability. That made CLI behavior easy to implement, but it pulled transport semantics into handler code and broke the symmetry with the rest of the runtime API, where handlers only depend on transport-independent request/response abstractions.

We want handlers to return ordinary Go errors by default while still allowing CLI and future transports to produce structured error outputs, exit-code mappings, and retryability hints.

## Decision

- Remove the public `procframe.Error` interface.
- Introduce `procframe.Status` as the transport-facing metadata struct with `Code`, `Message`, and `Retryable`.
- Keep `procframe.StatusError` as the structured error wrapper type, but store and expose `*Status` so transports can pass status metadata without per-call struct copies.
- Introduce `procframe.ErrorMapper` so transports can classify plain errors at the boundary and return `*Status`.
- No mapper is set by default. The application must explicitly provide an `ErrorMapper` to classify errors at the boundary.
- CLI uses an `ErrorMapper` to decide structured error output and exit-code semantics, and wraps mapped plain errors into `StatusError` before returning them to callers.

## Consequences

- Handler implementations can stay on ordinary Go `error` unless they intentionally choose a procframe status.
- Transport-specific classification policy lives at the boundary instead of in handler contracts.
- User-defined error types no longer become structured procframe errors merely by implementing a matching method set; applications must map them explicitly.
- Future transports can reuse the same `Status`/`ErrorMapper` model without requiring handlers to depend on transport semantics.
- Boundary code and structured error extraction avoid repeated `Status` copies on hot paths.
