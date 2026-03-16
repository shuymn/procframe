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
  - Status: `resolved`
  - Decision: IDEA.md §17.1 ドラフト採用 + error frame に `retryable` 追加
    - inbound: `{"id":"...","procedure":"/pkg.Service/Method","payload":{...}}`
    - outbound success: `{"id":"...","payload":{...},"eos":true|false}`
    - outbound error: `{"id":"...","error":{"code":"...","message":"...","retryable":false},"eos":true}`
    - `error` と `payload` は排他。`error` 時は常に `eos: true`
  - Design decisions:
    - JSON text frames (protojson 一貫性、デバッグ容易)。バイナリは将来 subprotocol ネゴで追加可能
    - payload は `json.RawMessage` として受け取り、dispatch 先でプロトコル固有の型にアンマーシャル
    - リクエストレベルエラー: インバンドのエラーフレーム。コネクションレベルエラー: WS close code (1000/1008/1011/4000/4001)

- Question: WS frame ID の生成責務
  - Class: `risk-bearing`
  - Resolution: `decision`
  - Status: `resolved`
  - Decision: クライアント生成。サーバーは ID をそのままレスポンスに反映する。ID 衝突時の挙動は未定義（クライアント責務）。

- Question: WS グレースフルシャットダウンのシーケンス
  - Class: `risk-bearing`
  - Resolution: `decision`
  - Status: `resolved`
  - Decision: 初期実装は基本 close のみ（`conn.CloseNow()`）。drain-with-deadline は将来の拡張。context cancel により handler は停止する。

- Question: WS バックプレッシャー制御
  - Class: `risk-bearing`
  - Resolution: `decision`
  - Status: `resolved`
  - Decision: write channel のバッファサイズ (default 64) + semaphore による max-inflight 制御 (default 64)。超過時は CodeUnavailable + retryable=true で即時拒否。

- Question: WsOptions の将来拡張
  - Class: `non-risk-bearing`
  - Resolution: `decision`
  - Status: `deferred`
  - Reason: 現状は `enabled` のみで十分

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

- [x] Theme: WS Transport Spike
  - Outcome: `github.com/coder/websocket` を使った WS transport の feasibility が実証される
  - Goal: 4 つの spike テストで基本フレーム往復、protojson payload ラウンドトリップ、並行 write 直列化、切断時 context キャンセル伝播を検証
  - Must Not Break: 既存テスト、go.mod の既存依存
  - Non-goals: WS transport のランタイム実装、codegen、production API
  - Acceptance (EARS):
    - When a JSON text frame is sent over a WS connection, the server shall parse the envelope and echo the payload back
    - When a proto message is marshalled with protojson and embedded as json.RawMessage in a JSON envelope, round-trip shall preserve all fields
    - When 3 goroutines concurrently send frames through a write channel, the WS connection shall receive all frames without race detector warnings
    - When a client closes a WS connection, the server-side context shall be cancelled within 1 second
    - When a WS frame arrives with a registered procedure, the server shall dispatch to the procframe unary handler and return the serialized response
    - When a server-stream handler sends multiple responses, the server shall emit frames with eos=false followed by a final eos=true frame
    - When a handler returns a StatusError, the server shall construct an error frame with the correct code, message, and retryable flag
    - When multiple requests arrive on a single connection with different IDs, the server shall dispatch them concurrently and correlate responses by ID
    - When the inflight request count exceeds max-inflight, the server shall reject with CodeUnavailable and retryable=true
  - Evidence: `run=go test -race -count=1 ./transport/ws/...; oracle=全 spike テスト pass、race detector 警告なし; visibility=independent; controls=[agent,context]; missing=[]; companion=none`
  - Gates: `spike`

- [x] Theme: WS Transport Runtime + Codegen
  - Outcome: proto 定義から生成されたサービスが WebSocket 経由で Unary / Server Streaming RPC を処理できる
  - Goal: transport/ws ランタイムパッケージ実装、codegen による New{Service}WSHandler 生成
  - Must Not Break: 既存 handler interface の signature, CLI/Connect transport の動作, codegen の既存出力, error code 体系, proto option の後方互換性
  - Non-goals: WS クライアント実装, バイナリフレーム, interceptor/middleware, ping/pong heartbeat, compression, reconnection, Shutdown の drain-with-deadline (基本 close のみ)
  - Acceptance (EARS):
    - When a method has ws.enabled = true, the codegen shall register it in New{Service}WSHandler
    - When a method has ws.enabled = false (default), the codegen shall exclude it
    - When no methods have ws.enabled = true, the codegen shall not produce a WS handler and not import transport/ws
    - When a WS client sends an inbound frame for a registered unary procedure, the Server shall return the response as an outbound frame with eos=true
    - When a WS client sends an inbound frame for a server-streaming procedure, the Server shall emit data frames (eos=false) followed by a final eos=true
    - When a handler returns a StatusError, the outbound frame shall carry error with correct code, message, retryable
    - When WithErrorMapper is configured, non-StatusError shall be classified before error frame construction
    - When multiple requests arrive with different IDs, the Server shall dispatch concurrently and correlate by ID
    - When inflight exceeds max-inflight, the Server shall reject with CodeUnavailable + retryable=true
    - When a client sends an unknown procedure, the Server shall respond with CodeNotFound error frame
    - When a client disconnects, the server-side context shall be cancelled
  - Evidence: `run=task check; oracle=generated WS handler serves unary/streaming, error codes map correctly, opt-out excluded, multiplex+inflight work; visibility=independent; controls=[agent,context]; missing=[]; companion=none`
  - Gates: `static`, `integration`
  - Executable doc: TestIntegration_WSUnarySuccess, TestIntegration_WSUnaryError, TestIntegration_WSServerStreaming, TestIntegration_WSOptOut, TestIntegration_WSMultiplexed, TestIntegration_WSMaxInflight, TestIntegration_WSDisconnect
  - Why not split vertically further?: runtime と codegen は同時に機能して初めて WS RPC が成立する (Connect Theme と同じ理由)
  - Escalate if: proto.Message 型制約と procframe の any 型パラメータの間で型安全なアダプタが作れない場合

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
