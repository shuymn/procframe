# KV Quickstart

In-memory key-value store demonstrating unary and server-stream RPC shapes.

## RPC

| Command | Shape | Description |
|---------|-------|-------------|
| `kv get --key <key>` | unary | Get a value by key. Returns `NotFound` error for missing keys |
| `kv list --prefix <prefix>` | server-stream | Stream entries matching the given prefix |

## Usage

```bash
# Build & run
make run

# Unary: get a value
go run . kv get --key greeting

# Unary: missing key → NotFound error
go run . kv get --key missing

# Server-stream: all entries
go run . kv list

# Server-stream: prefix filter
go run . kv list --prefix greeting

# JSON input
go run . --json '{"key":"greeting"}' kv get

# Compact JSON output
go run . --output json kv get --key greeting

# NDJSON stream output
go run . --output json kv list

# Help
go run . kv --help

# Show schema
go run . schema
```
