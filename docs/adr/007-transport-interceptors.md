# ADR-007: Transport-shared interceptors

## Context

CLI, Connect, and WebSocket transports each invoked handlers directly at their own boundary. That kept each transport simple, but it left no shared place for cross-cutting behavior such as tracing, metrics, logging, authorization checks, or short-circuiting. A Runner-only `before/after` hook would cover CLI, but it would split the public extension model across transports and force applications to learn different concepts for the same concern.

We want one interceptor contract that works across all server-side transports while keeping handler signatures transport-independent and preserving the boundary-owned responsibilities already established by ADR-006.

## Decision

- Introduce a transport-shared interceptor contract in `procframe`.
- Model the contract after connect-go's interceptor shape: type-erased request/response views plus per-call wrapper functions instead of transport-specific hook callbacks.
- Expose `Meta` and `CallSpec` as values in the shared request/response/conn views. `Request` / `Response` stay pointer wrappers around `Msg *T`, but the small metadata structs are cheaper overall as values than as separately allocated pointers on the interceptor hot path.
- Support three wrapper points:
  - unary call execution
  - server-stream call execution
  - individual server-stream `Send`
- Keep transport-specific classification at the boundary. Interceptors run before `ErrorMapper` / Connect code mapping / WS error-frame construction.
- Add `WithInterceptors(...procframe.Interceptor)` to CLI, Connect, and WS transport options.
- Keep `cli.Node.Run` unchanged. CLI carries the configured interceptor chain through execution context so generated commands can use the shared invoke helper without changing the hand-written Node contract.

## Rejected Alternatives

- Runner-only `before/after` hooks: rejected because they solve only CLI and create a second extension model unrelated to Connect and WS.
- Separate interceptor APIs per transport: rejected because the same handler graph would need three integration surfaces for the same concern.
- Transport-specific data exposure in the interceptor contract: rejected for v1 because it would leak HTTP/CLI/WS boundary details into a shared abstraction and make handlers/extensions less regenerable.

## Consequences

- Applications can attach one interceptor chain to any supported server-side transport.
- Generated CLI runners now execute through the same core invocation helpers as Connect and WS.
- Stream-level middleware can observe each outbound message without forcing handlers to change shape.
- CLI injects its interceptor chain through context internally, so hand-written `cli.Node` usage does not need a signature migration.
- Interceptor-heavy call paths avoid extra metadata allocations while preserving `nil *Request` / `nil *Response` semantics through the outer pointer wrappers.

## Revisit trigger

- Revisit when client-side transports are introduced, or when applications need interceptor access to transport-native data such as HTTP headers, raw CLI argv, or raw WS frames.
