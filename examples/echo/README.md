# Echo Example

Unary RPC の最小構成。handler が message をそのまま返す。

## Usage

```bash
# ビルド & 実行
make run

# フラグ指定
go run . echo run --message hello

# JSON 入力
go run . --json '{"message":"from-json"}' echo run

# compact JSON 出力
go run . --output json echo run --message hello

# ヘルプ
go run . echo --help

# スキーマ表示
go run . schema
```
