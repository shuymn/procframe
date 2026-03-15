# Review Guide

Use this file only for code review or when the user explicitly asks for broader review conventions.

Prioritize behavior, safety, regressions, and missing tests over style.

| Viewpoint | Check | Red Flags |
|---|---|---|
| API and spec alignment | Compare doc comments, examples, defaults, fallback paths, and return-value contracts against the implementation. | Comments promise behavior the code does not implement; hidden fallback paths; surprising defaults; behavior that depends on caller knowledge not stated at the boundary. |
| Naming and API shape | Check whether names rely on surrounding context, whether cheap accessors avoid `Get`, and whether receiver naming and pointer-vs-value choice stay consistent. | Repetitive names such as `NewWidget`, `DeleteUser`, or `ValidateConfig`; inconsistent receiver naming; API shape that makes simple call paths harder than necessary. |
| Error handling | Check `%w` vs `%v`, boundary translation, exported return types, and whether callers retain control over logging. | Functions that both log and return the same error; `%w` exposing internals accidentally; concrete error return types in exported APIs; in-band error signaling in a single return value. |
| Control flow | Check unusual branches, cleanup paths, and behavior that depends on implicit state. | Easy-to-misread branches without comments; fallback behavior that changes meaning depending on environment or current state; cleanup paths that are missing or asymmetric. |
| Concurrency | Check goroutine shutdown, channel ownership, shared mutable state, and whether concurrency guarantees are documented when non-obvious. | Goroutines without a clear stop path; unclear channel close ownership; asynchronous APIs where synchronous ones would be simpler; locking rules that are hard to infer from the API. |
| Resource lifecycle | Check open/close, cancel/release, and ownership transfer at API boundaries. | Resources that callers must clean up but the contract does not say so; background work that outlives the request; hidden ownership transfer. |
| Tests | Check coverage of error paths, regressions, isolation, and diagnosability of failures. | Missing tests for branchy or risky behavior; `t.Fatal` or `t.FailNow` from helper goroutines; deep mocks where real boundaries are available; failure messages that do not make `got` vs `want` obvious. |
