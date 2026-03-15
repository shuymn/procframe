# Ticker Example

Server Streaming RPC。handler が count 回 stream.Send する。

## Usage

```bash
# ビルド & 実行
make run

# フラグ指定
go run . ticker run --prefix tick --count 3

# NDJSON 出力
go run . --output json ticker run --prefix tick --count 3

# JSON 入力
go run . --json '{"prefix":"ping","count":5}' ticker run

# ヘルプ
go run . ticker --help

# スキーマ表示
go run . schema
```
