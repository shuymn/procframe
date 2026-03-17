# procframe

Proto-driven typed handler runtime for Go. Define procedures in Protocol Buffers, generate transport glue, and write only the handler.

One handler serves CLI, Connect/gRPC, and WebSocket. The same generic middleware model covers all 4 RPC shapes: unary, client-stream, server-stream, and bidi.

## Key Features

- Use `.proto` files as the single source of truth for procedure schemas and config.
- Run the same typed handler on CLI, Connect/gRPC, and WebSocket.
- Support all 4 RPC shapes: unary, client-stream, server-stream, and bidirectional streaming.
- Compose conn-based middleware around a single `Conn` interface with `Receive` and `Send`.
- Generate handler interfaces, the CLI command tree, flag parsing, Connect/gRPC handlers, WebSocket session handlers, and config loading.
- Support both human and agent workflows with flat flags for humans and `--json` or stdin NDJSON for agents.
- Map canonical error codes to exit codes, Connect/gRPC status codes, and WebSocket error frames.
- Keep result data on stdout and diagnostics on stderr.

## Transports

- CLI uses flags or `--json` for unary and server-stream procedures, and stdin NDJSON for client-stream and bidi procedures.
- Connect/gRPC runs over HTTP. Bidi requires HTTP/2.
- WebSocket uses a JSON session protocol with `open`, `message`, and `close` frames.

All transports support all 4 RPC shapes (unary, client-stream, server-stream, bidi).

## Installation

```bash
go get github.com/shuymn/procframe
```

Code generation plugin:

```bash
go install github.com/shuymn/procframe/cmd/protoc-gen-procframe-go@latest
```

Add to your `buf.gen.yaml`:

```yaml
version: v2
plugins:
  - local: protoc-gen-go
    out: gen
    opt: paths=source_relative
  - local: protoc-gen-procframe-go
    out: gen
    opt:
      - paths=source_relative
      # - config_proto=path/to/config.proto  # default: any file named config.proto
```

### Requirements

- Go 1.25+
- [buf](https://buf.build/) or `protoc` (for proto code generation)

## Contributing

Development requires additional tooling:

- [Task](https://taskfile.dev/) is the task runner. Use `task check` for full verification.
- [lefthook](https://github.com/evilmartians/lefthook) manages Git hooks. Run `lefthook install` after cloning.

```bash
task              # list all available tasks
task check        # lint + proto + build + test + tidy
task test         # test with race detection
```

## License

[MIT](LICENSE)
