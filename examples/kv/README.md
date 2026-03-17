# KV Quickstart

In-memory key-value store で unary RPC と server-stream RPC を示す。

## RPC

| Command | Shape | 概要 |
|---------|-------|------|
| `kv get --key <key>` | unary | キーで値を取得。未存在は `NotFound` エラー |
| `kv list --prefix <prefix>` | server-stream | prefix マッチするエントリをストリーム返却 |

## Usage

```bash
# ビルド & 実行
make run

# unary: 値を取得
go run . kv get --key greeting

# unary: 存在しないキー → NotFound エラー
go run . kv get --key missing

# server-stream: 全エントリ
go run . kv list

# server-stream: prefix フィルタ
go run . kv list --prefix greeting

# JSON 入力
go run . --json '{"key":"greeting"}' kv get

# compact JSON 出力
go run . --output json kv get --key greeting

# NDJSON ストリーム出力
go run . --output json kv list

# ヘルプ
go run . kv --help

# スキーマ表示
go run . schema
```
