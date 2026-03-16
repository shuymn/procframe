# procframe

A proto-driven typed runtime for CLI frameworks in Go.

Define your config and procedures in Protocol Buffers, and `procframe` generates the CLI glue code. You write only the handler.

## Overview

`procframe` connects two concerns with minimal abstraction:

1. **Config**: env / CLI / file → merge → immutable config
2. **Procedure**: CLI → typed request → handler → typed response

The runtime provides transport-independent abstractions (`Request[T]`, `Response[T]`, `ServerStream[T]`). Handlers return ordinary Go errors; transports map them to structured statuses at the boundary. A protoc plugin (`protoc-gen-procframe-go`) generates CLI command trees, flag parsers, and config loaders from your proto definitions.

### Key Features

- **Proto as single source of truth** — config and procedure schemas defined in `.proto` files
- **Code generation** — CLI command tree, flag parsing, config loading, handler interfaces
- **Transport-independent handlers** — handlers know nothing about CLI, HTTP, or WebSocket
- **Human + agent dual interface** — flat flags for humans, `--json` raw payload for agents
- **Schema introspection** — `schema` subcommand for self-describing CLI
- **Structured errors** — canonical error codes with exit code mapping and retryable hints
- **stdout/stderr separation** — result data on stdout, logs/help/errors on stderr

## Status

v0.1 — CLI transport only. HTTP, gRPC, and WebSocket transports are planned but not yet implemented.

Supported handler shapes: **unary** and **server-stream**.

## Requirements

- Go 1.25+
- [Task](https://taskfile.dev/)
- [buf](https://buf.build/) (for proto code generation)
- [lefthook](https://github.com/evilmartians/lefthook) (for Git hooks)

## Setup

```bash
lefthook install
```

## Development

```bash
task          # list all available tasks
task build    # build
task test     # test with race detection
task lint     # lint
task fmt      # format
task check    # lint + build + test
```

## Repository Layout

```
procframe/
├── cmd/protoc-gen-procframe-go/  # protoc plugin
├── internal/codegen/             # code generation logic
├── proto/procframe/options/v1/   # options.proto (library-provided schema DSL)
├── transport/cli/                # CLI transport runtime
├── doc.go                        # package documentation
├── request.go                    # Request[T]
├── response.go                   # Response[T]
├── stream.go                     # ServerStream[T]
├── meta.go                       # Meta (procedure, request ID, session ID)
├── errors.go                     # Structured error with canonical codes
└── handler.go                    # UnaryHandler / ServerStreamHandler interfaces
```

## License

[MIT](LICENSE)
