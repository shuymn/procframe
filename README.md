# procframe

Proto-driven typed handler runtime for Go. Define procedures in Protocol Buffers, generate transport glue, write only the handler.

One handler serves CLI, Connect (HTTP), and WebSocket — all four RPC shapes (unary, client-stream, server-stream, bidi) through a single generic middleware model.

## Key Features

- **Proto as single source of truth** — procedure schemas and config defined in `.proto` files
- **One handler, three transports** — same typed handler runs on CLI, Connect/gRPC, and WebSocket
- **All four RPC shapes** — unary, client-stream, server-stream, bidirectional streaming
- **Generic conn-based middleware** — single `Conn` interface with `Receive`/`Send` for all shapes; compose behavior via conn decorators
- **Code generation** — handler interfaces, CLI command tree, flag parsing, Connect HTTP handlers, WebSocket session handlers, config loading
- **Human + agent dual interface** — flat flags for humans, `--json` / stdin NDJSON for agents
- **Structured errors** — canonical error codes mapped to exit codes, Connect codes, WS error frames
- **stdout/stderr separation** — result data on stdout, diagnostics on stderr

## Transports

- **CLI** — flags or `--json` for unary/server-stream, stdin NDJSON for client-stream/bidi
- **Connect** — Connect protocol and gRPC over HTTP (bidi requires HTTP/2)
- **WebSocket** — JSON session protocol (`open`/`message`/`close` frames)

All transports support all four RPC shapes (unary, client-stream, server-stream, bidi).

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

- [Task](https://taskfile.dev/) — task runner (`task check` for full verification)
- [lefthook](https://github.com/evilmartians/lefthook) — Git hooks (`lefthook install` after clone)

```bash
task              # list all available tasks
task check        # lint + proto + build + test + tidy
task test         # test with race detection
```

## License

[MIT](LICENSE)
