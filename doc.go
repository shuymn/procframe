// Package procframe provides a proto-driven typed runtime for CLI frameworks.
//
// It defines transport-independent abstractions for requests, responses,
// streaming, handlers, and structured errors. Code generation from proto
// definitions produces the CLI glue code; this package supplies the minimal
// runtime kernel that the generated code builds on.
package procframe
