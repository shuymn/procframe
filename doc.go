// Package procframe provides a proto-driven typed handler runtime.
//
// It defines transport-independent abstractions for requests, responses,
// streaming, and handlers across all four RPC shapes (unary, client-stream,
// server-stream, bidi). Procedure handlers return ordinary Go errors;
// transports map those errors to structured statuses at the boundary.
// Code generation from proto definitions produces handler interfaces and
// transport glue for CLI, Connect/gRPC (HTTP), and WebSocket; this package
// supplies the minimal runtime kernel that the generated code builds on.
package procframe
