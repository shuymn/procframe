# Greeter Example

Config 管理デモ。prefix/suffix を環境変数・bootstrap フラグ・デフォルト値で解決する。

## Usage

```bash
# ビルド & 実行 (デフォルト: "Hello World!")
make run

# デフォルト設定
go run . greet run --name World

# 環境変数で prefix を変更
GREETER_PREFIX=Hi go run . greet run --name World

# bootstrap フラグで prefix を変更
go run . --prefix Hey greet run --name World

# suffix も環境変数で変更
GREETER_PREFIX=Hi GREETER_SUFFIX='!!' go run . greet run --name World

# ヘルプ
go run . greet --help

# スキーマ表示
go run . schema
```

## Config 解決順序

1. デフォルト値 (`prefix="Hello"`, `suffix="!"`)
2. 環境変数 (`GREETER_PREFIX`, `GREETER_SUFFIX`)
3. bootstrap フラグ (`--prefix`、bootstrap=true のフィールドのみ)
