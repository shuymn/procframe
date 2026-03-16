// Package procframe provides a proto-driven typed runtime for CLI frameworks.
//
// It defines transport-independent abstractions for requests, responses,
// streaming, and handlers. Procedure handlers return ordinary Go errors;
// transports map those errors to structured statuses at the boundary for
// exit-code mapping, JSON error output, and similar transport concerns.
// Code generation from proto definitions produces the CLI glue code; this
// package supplies the minimal runtime kernel that the generated code builds on.
package procframe
