# TODO

## Open Questions

- Question: CLI field-to-flag 変換規則
  - Class: `risk-bearing`
  - Resolution: `decision`
  - Status: `resolved`
  - Decision:
    - flag naming: snake_case → kebab-case (`log_level` → `--log-level`)
    - enum: 型名プレフィックス除去 + lowercase, case-insensitive 受付 (`PULL_REQUEST_STATE_OPEN` → `open`)。strip 後に値が衝突したら codegen error
    - repeated: 複数回指定 (`--tag foo --tag bar`)
    - nested message (non-bind_into): flat flag 生成なし、`--json` fallback のみ

## Deferred Questions

- Question: WS frame の正式 schema
  - Class: `risk-bearing`
  - Resolution: `decision`
  - Status: `deferred`
  - Reason: WS は v0.1 スコープ外
  - Decision: IDEA.md §17.1 ドラフト採用 + error frame に `retryable` 追加
    - inbound: `{"id":"...","procedure":"/pkg.Service/Method","payload":{...}}`
    - outbound success: `{"id":"...","payload":{...},"eos":true|false}`
    - outbound error: `{"id":"...","error":{"code":"...","message":"...","retryable":false},"eos":true}`
    - `error` と `payload` は排他。`error` 時は常に `eos: true`

## Themes

- [x] Theme: Runtime core + options.proto
  - Outcome: procframe パッケージが Go モジュールとしてコンパイル可能になり、options.proto が protoc で import 可能になる
  - Goal: Request[T], Response[T], ServerStream[T], Meta, Error, Code 型と options.proto の提供
  - Must Not Break: なし (新規プロジェクト)
  - Non-goals: codegen, transport 実装, config 実装
  - Acceptance (EARS):
    - When `go build ./...` を実行すると procframe パッケージがエラーなくコンパイルされる
    - When options.proto に対して lint を実行すると valid と判定される
    - When 外部 proto ファイルが options.proto を import して `protoc` を実行すると成功する
  - Evidence: `run=task check; oracle=compile success + lint pass; visibility=independent; controls=[agent,context]; missing=[]; companion=none`
  - Gates: `static`
  - Executable doc: `go build ./...` passes; options.proto が protoc で正常に処理される
  - Why not split vertically further?: Runtime types と options.proto は相互に意味を持ち、片方だけでは外部から観測可能な前進にならない
  - Escalate if: proto2 extension の `retention = RETENTION_SOURCE` が protoc バージョン依存で動かない場合

- [x] Theme: Codegen + CLI unary end-to-end
  - Outcome: ユーザーが service proto を書き、protoc-gen-procframe-go を実行し、handler を実装し、CLI unary コマンドを flat flags で実行できる
  - Goal: protoc-gen-procframe-go による handler interface + CLI runner 生成、CLI unary 実行の end-to-end 動作
  - Must Not Break: Theme 1 の public API (Request, Response, ServerStream, Meta, Error, Code)
  - Non-goals: server-stream CLI, --json 入力, schema コマンド, --output json, WS codegen, config codegen
  - Acceptance (EARS):
    - When テスト用 service proto に対して `protoc --procframe-go_out=...` を実行すると handler interface と CLI runner のコードが生成される
    - When 生成コードを含むパッケージに対して `go build` を実行するとコンパイルが成功する
    - When 生成された CLI runner で unary コマンドを flat flags 付きで実行すると handler が呼ばれ結果が stdout に出力される
    - When group flags (bind_into) を指定すると対応する request field に値が注入される
    - When enum field の strip 後の値が衝突する proto を codegen すると error になる
    - When help を表示すると stderr に出力される
  - Evidence: `run=go test ./internal/codegen/... && go test ./transport/cli/...; oracle=integration test pass; visibility=independent; controls=[agent,context]; missing=[]; companion=none`
  - Gates: `static`, `integration`
  - Executable doc: fixture proto → protoc-gen-procframe-go → compile → CLI 実行 → 期待出力検証の integration test
  - Why not split vertically further?: handler interface 生成だけでは transport なしに動作確認できず、外部から観測可能な前進にならない。codegen と CLI transport は最初の vertical slice として不可分
  - Escalate if: protoc plugin API (protogen) で procframe custom option の読み取りに制約がある場合

- [x] Theme: CLI server-stream + agent features
  - Outcome: CLI で server-stream が動作し、agent 向け機能 (--json, schema, --output json, 構造化 error, exit code) が使える
  - Goal: server-stream CLI 出力、--json raw payload 入力、schema サブコマンド、--output json 構造化出力、exit code mapping、構造化 error 出力
  - Must Not Break: Theme 2 の unary CLI 動作 (flat flags + text output)
  - Non-goals: NDJSON pagination, field mask, --dry-run, --sanitize, MCP, batch/stdin pipe, error の suggested_action/failed_input
  - Acceptance (EARS):
    - When server-stream handler を CLI で実行すると chunk ごとに stdout に出力される
    - When `--json '{...}'` を指定すると protojson が直接 unmarshal されて request になる
    - When `--json` と flags を同時指定すると error が返る
    - When `app schema <procedure-path>` を実行すると request/response の型情報が JSON で stdout に出力される
    - When `--output json` を指定すると response が protojson で stdout に出力される
    - When `--output json` で server-stream を実行すると NDJSON (1 chunk 1 行) で出力される
    - When handler が Error を返すと対応する exit code で終了する
    - When `--output json` で error が発生すると stderr に `{"error":{"code":"...","message":"...","retryable":...}}` が出力される
  - Evidence: `run=go test ./transport/cli/...; oracle=integration test pass; visibility=independent; controls=[agent,context]; missing=[]; companion=none`
  - Gates: `static`, `integration`
  - Executable doc: --json 入力 → 期待 request 検証, schema → 期待 JSON 検証, --output json → 期待出力検証, server-stream → NDJSON 検証, error → exit code + stderr JSON 検証の integration test
  - Why not split vertically further?: --json, schema, --output json は agent workflow として一体で使われ、個別提供では agent が self-discovery → structured I/O の流れを完結できない
  - Escalate if: proto descriptor から JSON Schema 相当の型情報を codegen 時に静的生成する方法に制約がある場合

- [ ] Theme: Config system
  - Outcome: ユーザーが config.proto を定義し、JSON file + env + bootstrap CLI flags から immutable config を生成できる
  - Goal: config.proto からの codegen (LoadRuntimeConfig 関数生成)、defaults → file(JSON) → env → bootstrap CLI → validate → immutable の merge chain
  - Must Not Break: Theme 1 の public API; codegen plugin の既存生成物
  - Non-goals: YAML/TOML, config hot reload, config watch, 複数 config ファイル, config validation framework
  - Acceptance (EARS):
    - When config.proto に ConfigFieldOptions を付けたフィールドがあるとき protoc-gen-procframe-go が LoadRuntimeConfig 関数を生成する
    - When JSON config ファイル、環境変数、bootstrap CLI flags を組み合わせて LoadRuntimeConfig を呼ぶと defaults → file → env → bootstrap の優先順位で merge された config が返る
    - When `required=true` のフィールドが未設定のとき error が返る
    - When `bootstrap=true` のフィールドは bootstrap flags として parse され、残りの argv が procedure args として返る
    - When `secret=true` のフィールドはログ出力時にマスクされる
  - Evidence: `run=go test ./config/... && go test ./cmd/protoc-gen-procframe-go/...; oracle=integration test pass; visibility=independent; controls=[agent,context]; missing=[]; companion=none`
  - Gates: `static`, `integration`
  - Executable doc: fixture config.proto → codegen → JSON file + env vars + bootstrap flags → LoadRuntimeConfig → 期待 config 値検証の integration test
  - Why not split vertically further?: config の merge chain (defaults → file → env → bootstrap → validate) は各層が密結合しており、部分提供では config システムとして成立しない
  - Escalate if: bootstrap CLI の argv 分離ロジックで procedure args との境界が曖昧になる場合 (e.g. `--config` と procedure flag の衝突)

