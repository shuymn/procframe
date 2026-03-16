# TODO

## Open Questions

- Question: HTTP transport のスコープ: minimal JSON-over-HTTP vs Connect Protocol (+ gRPC)?
  - Class: `risk-bearing`
  - Resolution: `spike`
  - Status: `escalated`
  - Note: Connect なら HTTP + gRPC を同時に達成できるが、CLI の flat flag 構造 (bind_into 展開, repeated, enum 文字列マッチ) と gRPC のフル protobuf メッセージ受信は本質的に異なるインターフェース。同じ proto 定義から両方の transport コードを矛盾なく生成できるか要検証。

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

- [x] Theme: Help/Schema メタデータ伝播
  - Outcome: CLI の help と schema 出力が proto 定義のメタデータを表示する
  - Goal: proto コメントと enum 値を生成コードの help テキストと schema JSON に反映する
  - Must Not Break: 既存の flag パース動作, schema サブコマンドの JSON 構造の後方互換性, codegen の既存出力
  - Non-goals: 新しい proto option の追加, 新しい flag 型, help 以外の CLI 機能変更, nested/map の flat flag 化, config bootstrap flags の help 表示
  - Acceptance (EARS):
    - When a proto field has a leading comment, the generated flag usage string shall include it
    - When a field is an enum type, the help output shall list allowed values
    - When schema subcommand is run, the output shall include description and enum values for each field
    - When a bind_into group has fields with metadata, the group flags shall also show metadata
    - When a leaf command is invoked with --help, the help output shall show flag definitions with usage text
  - Evidence: `run=task check; oracle=generated help text contains proto comments and enum values; schema JSON contains description/enum; visibility=independent; controls=[agent,context]; missing=[]; companion=none`
  - Gates: `static`, `integration`
  - Executable doc: TestIntegration_HelpShowsFieldDescriptions, TestIntegration_HelpShowsEnumValues, TestIntegration_SchemaContainsDescription
  - Why not split vertically further?: 全メタデータ伝播は同一の codegen パス (field descriptor → flag registration + schema struct) を共有し、出力面も help text + schema JSON の2つだけ。メタデータ種別で分割すると同じコードパスを複数回変更することになる。
  - Escalate if: protogen API が proto ソースコメントの取得を十分にサポートしていない場合
