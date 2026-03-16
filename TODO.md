# TODO

## Open Questions

- Question: HTTP transport のスコープ: minimal JSON-over-HTTP vs Connect Protocol (+ gRPC)?
  - Class: `risk-bearing`
  - Resolution: `spike`
  - Status: `resolved`
  - Result: Connect Protocol は feasible。spike test (`transport/connect/spike_test.go`) で以下を実証:
    1. 同一 handler 実装が CLI と Connect の両方で動作 (TestSpike_Coexistence)
    2. procframe error code が gRPC/Connect status code に正しく写像 (TestSpike_UnaryError: 全 8 コード)
    3. server streaming が Connect でも動作 (TestSpike_ServerStreaming)
    4. CLI 専用 option (cli_group, bind_into, cli_path) が Connect transport に干渉しない (既存テスト regression なし)
  - Note: アダプタ層 (`adaptUnary`, `adaptServerStream`) は薄く、connect-go の低レベル API で codegen なしに実装可能。proto message type は CLI/Connect 間で共有される。

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

- [x] Theme: Connect Transport 基本対応
  - Outcome: proto 定義から生成されたサービスが Connect Protocol (HTTP) 経由で Unary / Server Streaming RPC を処理できる
  - Goal: proto option (`connect.enabled`) の追加、`transport/connect` ランタイムパッケージの実装、codegen による `New{Service}ConnectHandler` 生成
  - Must Not Break: 既存 handler interface の signature, CLI transport の動作, codegen の既存出力 (handler interface / CLI runner), error code 体系, proto option の後方互換性
  - Non-goals: Connect クライアント生成, interceptor/middleware 対応, connect.HandlerOption パススルー, gRPC reflection, Connect request header → Meta mapping, WS transport
  - Acceptance (EARS):
    - When a method has `connect.enabled = true`, the codegen shall include that method in the `New{Service}ConnectHandler(h Handler, opts...) (string, http.Handler)` function
    - When a method has `connect.enabled = false` (default), the codegen shall exclude that method from the Connect handler
    - When a Connect client sends a unary request to the generated handler, the handler shall invoke the procframe handler and return the serialized response
    - When a Connect client opens a server stream, the handler shall invoke the procframe stream handler and deliver each response as a stream message
    - When a handler returns a `StatusError`, the Connect response shall carry the mapped `connect.Code`
    - When a handler returns a non-StatusError and `WithErrorMapper` is configured, the mapper shall classify the error before code mapping
    - When no methods have `connect.enabled = true`, the codegen shall not produce a Connect handler function and shall not import `transport/connect`
  - Evidence: `run=task check; oracle=generated Connect handler serves unary/streaming RPCs correctly, error codes map to correct connect.Code, opt-out method is excluded; visibility=independent; controls=[agent,context]; missing=[]; companion=none`
  - Gates: `static`, `integration`
  - Executable doc: TestIntegration_ConnectUnarySuccess, TestIntegration_ConnectUnaryError, TestIntegration_ConnectServerStreaming, TestIntegration_ConnectOptOut
  - Why not split vertically further?: runtime パッケージと codegen は同時に機能して初めて外部観測可能な前進になる。proto option, runtime, codegen は単一の handler 呼び出しパスを構成し、いずれか単体では Connect RPC が成立しない。
  - Escalate if: connect-go の generic handler API が procframe の handler function signature と型制約の間で型安全なアダプタを作れない場合

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
