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

- Question: Connect クライアント生成のランタイム配置: codegen のみ vs transport/connect にランタイム helper 追加
  - Class: `blocking`
  - Resolution: `decision`
  - Status: `resolved`
  - Decision: codegen のみ。Connect クライアントは `connectrpc.com/connect` の `Client[Req, Res]` を直接使えばよく、procframe 独自のランタイムラッパーは不要。生成コードは `connectrpc.com/connect` の型を直接参照する。

- Question: クライアントインターフェースの型: `connectrpc.com/connect` の型を直接使うか `procframe.Request`/`procframe.Response` でラップするか
  - Class: `blocking`
  - Resolution: `decision`
  - Status: `resolved`
  - Decision: `connectrpc.com/connect` の型を直接使用。クライアントはトランスポート固有であり procframe ラッパーで包む利点がない。Connect エコシステムとの互換性を優先する。

- Question: クライアントコンストラクタの命名: `New{Service}Client` vs `New{Service}ConnectClient`
  - Class: `risk-bearing`
  - Resolution: `decision`
  - Status: `resolved`
  - Decision: `New{Service}ConnectClient`。サーバー側 `New{Service}ConnectHandler` と対称。将来 WS クライアントを追加した場合に名前空間が衝突しない。

- Question: WS クライアントのインターフェース型: raw proto vs wrapper
  - Class: `blocking`
  - Resolution: `decision`
  - Status: `resolved`
  - Decision: raw proto。WS に wrapper 型を作る利点が薄い。最もエルゴノミック。

- Question: WS クライアントの Connection ownership: caller 提供 vs 内部管理
  - Class: `blocking`
  - Resolution: `decision`
  - Status: `resolved`
  - Decision: caller 提供 (`*ws.Conn`)。Connect client と同様、caller がライフサイクルを管理。

- Question: `Conn` vs `ClientConn` 命名
  - Class: `non-blocking`
  - Resolution: `decision`
  - Status: `resolved`
  - Decision: `Conn`。サーバー側は `Server` であり、同パッケージ内で `Conn` は明確。

- Question: Adversarial verification v1 pre-release — フェーズ順序
  - Class: `risk-bearing`
  - Resolution: `decision`
  - Status: `resolved`
  - Decision: risk-first 順序 (WS → CLI → Config → Connect → Core → Codegen)。最大攻撃表面を先に検査し修正時間を最大化する。フェーズ間に依存はなく独立実行可能。

- Question: Adversarial verification v1 pre-release — 検査対象スコープ
  - Class: `risk-bearing`
  - Resolution: `decision`
  - Status: `resolved`
  - Decision: 実装コードのみ。generated files (internal/gen/, examples/gen/)、examples/、cmd/protoc-gen-procframe-go/main.go (薄い entry point) は対象外。WS client (新規追加分) は WS フェーズに含める。

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

- [x] Theme: CLI schema メタデータ静的化
  - Outcome: generated CLI の `schema` 実行時に不変メタデータ再構築と runtime sort が発生しない
  - Goal: generator が schema 用データをサービス単位の静的変数として出力し、lookup/list 両方で再利用する
  - Must Not Break: `schema` JSON の形、command path lookup、procedure lookup、出力順、既存 integration test
  - Non-goals: `fmt.Fprintln(stdout, string(out))` 最適化、他の CLI 実行パス最適化、help/system/connect/WS 変更
  - Acceptance (EARS):
    - When a generated CLI runner serves the `schema` subcommand, it shall read from pre-generated service-level schema variables instead of rebuilding the metadata map at runtime
    - When `schema` is invoked without args, it shall return the same command list in command-path order as before
    - When `schema` is invoked with a command path, it shall return the same command metadata as before
    - When `schema` is invoked with a procedure path, it shall return the same command metadata as before
    - When schema benchmarks are replayed, allocs/op shall be lower than the pre-change baseline for both list and procedure lookup
  - Evidence: `run=task check && go test -run '^$' -bench 'BenchmarkSchema(List|LookupByProcedure)$' -benchmem ./transport/cli; oracle=schema integration tests keep behavior unchanged and benchmarks show lower allocs/op than the pre-change baseline (list: 12 allocs/op, lookup: 10 allocs/op); visibility=implementation-visible; controls=[]; missing=[agent,context]; companion=independent AI review of the current diff confirms no schema contract regression or additional blocker beyond trust metadata`
  - Gates: `static`, `integration`, `benchmark`
  - Executable doc: `TestIntegration_Schema`, `TestIntegration_SchemaSpecificCommand`, `TestIntegration_SchemaFallbackByProcedure`, `TestIntegration_SchemaStreamingFlag`, `TestIntegration_SchemaContainsDescription`, `BenchmarkSchemaList`, `BenchmarkSchemaLookupByProcedure`
  - Why not split vertically further?: schema list と lookup は同一の generated metadata source を共有しており、片方だけ静的化しても runtime map 再構築を除去したとは言えないため
  - Escalate if: service-level schema metadata を静的変数として生成すると Go の初期化順や generated file size の制約により既存 codegen が安全にコンパイルできなくなる場合

- [x] Theme: Transport 共通 Interceptor 対応
  - Outcome: アプリケーションが 1 つの interceptor API で CLI / Connect / WS の handler 実行前後と stream 送信を横断的に制御できる
  - Goal: `procframe` に共通 interceptor 契約と chain 実行 helper を追加し、各 transport option から適用できるようにする
  - Must Not Break: 既存 handler interface の signature, CLI/Connect/WS の既存成功系と error mapping, generated constructor 名, `ErrorMapper` の責務, 既存 schema/help 出力
  - Non-goals: transport 固有 hook API の追加, HTTP header や CLI raw args への直接アクセス, client-side interceptor, unary 以外の新しい handler 形, metrics/logging 実装そのもの
  - Acceptance (EARS):
    - When a transport is configured with `WithInterceptors`, the handler invocation shall pass through the registered interceptor chain before the underlying handler runs
    - When multiple interceptors are registered, the first registered interceptor shall be the outermost wrapper
    - When no interceptor is configured, the transport shall preserve the current behavior
    - When a unary interceptor short-circuits, the underlying handler shall not run and the returned response or error shall flow through the existing transport boundary behavior
    - When a server-stream interceptor wraps a call, it shall observe the stream handler lifecycle and each `Send`
    - When an interceptor returns an error, existing transport-specific error mapping shall still determine CLI exit/status output, Connect code mapping, and WS error frame construction
    - When a handler returns or sends `nil`, the shared invocation layer shall preserve the current internal-error semantics
  - Evidence: `run=task check; oracle=interceptor chain applies consistently across CLI/Connect/WS, short-circuit and stream send wrapping work, and existing boundary behavior remains unchanged; visibility=implementation-visible; controls=[]; missing=[agent,context]; companion=go test ./transport/... replay confirms transport-level behavior on generated CLI, Connect, and WS paths`
  - Gates: `static`, `integration`
  - Executable doc: `TestInvokeUnaryInterceptors`, `TestInvokeServerStreamInterceptors`, `TestIntegration_CLIInterceptor`, `TestIntegration_ConnectInterceptor`, `TestIntegration_WSInterceptor`
  - Why not split vertically further?: 共通 interceptor 契約、transport option、generated CLI 呼び出し経路は同じ public capability を構成しており、どれか単体では外部から観測できる前進にならないため
  - Escalate if: type-erased interceptor 契約と typed handler の間で unary/stream の両方を unsafe なしに橋渡しできない場合

- [x] Theme: Connect クライアントコード生成
  - Outcome: `connect.enabled = true` のメソッドに対し、型付き Connect クライアントが codegen で自動生成され、手動の `connectrpc.NewClient` 構築が不要になる
  - Goal: `protoc-gen-procframe-go` に Connect クライアント生成を追加。インターフェース、実装構造体、コンストラクタを `.proc.go` ファイルに出力する
  - Must Not Break: 既存 handler interface の signature, CLI/Connect/WS transport の動作, codegen の既存出力, error code 体系, proto option の後方互換性
  - Non-goals: WS クライアント生成, procframe ラッパー型でのクライアント型抽象化, クライアント側 interceptor, retry/reconnection ロジック, 新しい proto option の追加
  - Acceptance (EARS):
    - When a method has `connect.enabled = true`, the codegen shall include that method in the `{Service}ConnectClient` interface and `New{Service}ConnectClient` constructor
    - When a method has `connect.enabled = false` (default), the codegen shall exclude that method from the Connect client
    - When no methods have `connect.enabled = true`, the codegen shall not produce a Connect client interface, constructor, or `connectrpc.com/connect` import
    - When `New{Service}ConnectClient` is called with an HTTP client and base URL, the returned client shall provide typed method calls for each Connect-enabled RPC
    - When a unary client method is called, it shall delegate to `connect.Client.CallUnary` with the correct procedure string
    - When a client-stream client method is called, it shall delegate to `connect.Client.CallClientStream`
    - When a server-stream client method is called, it shall delegate to `connect.Client.CallServerStream`
    - When a bidi client method is called, it shall delegate to `connect.Client.CallBidiStream`
  - Evidence: `run=task check; oracle=generated Connect client compiles, round-trip tests pass for all four RPC shapes, opt-out methods excluded from client interface; visibility=independent; controls=[agent,context]; missing=[]; companion=none`
  - Gates: `static`, `integration`
  - Executable doc: `TestIntegration_ConnectClientUnary`, `TestIntegration_ConnectClientClientStream`, `TestIntegration_ConnectClientServerStream`, `TestIntegration_ConnectClientBidi`, `TestIntegration_ConnectClientOptOut`
  - Why not split vertically further?: クライアントインターフェース、実装構造体、コンストラクタは同一の codegen パスを構成し、いずれか単体では Connect クライアントが使用可能にならないため
  - Escalate if: `connectrpc.com/connect` の `Client[Req, Res]` 型が shape ごとに異なる呼び出しメソッドを持つ設計により、単一の generated interface で全 shape を型安全に統一できない場合

- [x] Theme: WS Client Runtime + Codegen
  - Outcome: `ws.enabled = true` のメソッドに対し、型付き WS クライアントが codegen で自動生成され、
    multiplexed WS 接続上で全 4 RPC shape を型安全に呼び出せる
  - Goal: transport/ws にクライアントランタイム (Conn, stream types, Call functions) を実装し、
    codegen による New{Service}WSClient 生成を追加する
  - Must Not Break: 既存 handler interface の signature, CLI/Connect/WS server transport の動作,
    codegen の既存出力, error code 体系, proto option の後方互換性
  - Non-goals: client-side interceptor (ADR-007 deferred), reconnection/retry, バイナリフレーム,
    ping/pong heartbeat, compression, 新しい proto option の追加
  - Acceptance (EARS):
    - When a method has ws.enabled = true, the codegen shall include that method in the
      {Service}WSClient interface and New{Service}WSClient constructor
    - When a method has ws.enabled = false (default), the codegen shall exclude that method
    - When no methods have ws.enabled = true, the codegen shall not produce a WS client
    - When New{Service}WSClient is called with a ws.Conn, the returned client shall provide
      typed method calls for each WS-enabled RPC
    - When a unary client method is called, it shall open a session, send the request,
      and return the single response or error
    - When a server-stream client method is called, it shall return a ServerStream
      whose Receive returns successive messages and io.EOF on close
    - When a client-stream client method is called, it shall return a ClientStream
      whose Send transmits messages and CloseAndReceive returns the response
    - When a bidi client method is called, it shall return a BidiStream
      supporting concurrent Send and Receive
    - When the server sends an error frame, the client shall surface it as
      *procframe.StatusError with the correct code, message, and retryable flag
    - When the WS connection is lost, all active sessions shall return an error
    - When the caller cancels ctx, the client shall send a cancel frame and unblock pending Receive
  - Evidence: `run=task check; oracle=generated WS client compiles, round-trip tests pass
    for all four RPC shapes, error frames → StatusError, disconnect propagates, cancel propagates;
    visibility=independent; controls=[agent,context]; missing=[]; companion=none`
  - Gates: `static`, `integration`
  - Executable doc: TestIntegration_WSClientUnary, TestIntegration_WSClientServerStream,
    TestIntegration_WSClientClientStream, TestIntegration_WSClientBidi,
    TestIntegration_WSClientError, TestIntegration_WSClientDisconnect,
    TestIntegration_WSClientCancel, TestIntegration_WSClientOptOut
  - Why not split vertically further?: runtime (Conn, stream types, Call functions) と
    codegen は同時に機能して初めて WS クライアント RPC が成立する
  - Escalate if: Go の type parameter 制約により package-level generic Call functions が
    protojson marshal/unmarshal と proto.Message constraint の間で型安全に橋渡しできない場合

- [x] Theme: Adversarial Verify — Transport WebSocket (Phase 1/6)
  - Outcome: transport/ws の攻撃耐性が Critical tier で検証され、脆弱性が文書化される
  - Goal: サーバー・クライアント・フレームプロトコル・セッション多重化・並行 dispatch・バックプレッシャーの攻撃耐性を検証
  - Target: transport/ws/server.go, client.go, call.go, client_stream.go, stream.go, frame.go, option.go, errors.go
  - Attack Categories: 1 (Input Boundary), 2 (Error Handling), 3 (Security Boundary), 4 (Concurrency), 5 (State & Data Integrity)
  - Must Not Break: 実装コード、既存テスト、attack-vectors.md
  - Non-goals: 脆弱性の修正 (別タスク)、generated files の直接検査
  - Acceptance (EARS):
    - When adversarial verification is executed at Critical tier, all [required] vectors within selected categories shall be probed or documented as N/A
    - When all probes result in DEFENDED and coverage gate passes, overall verdict shall be PASS
    - When a probe finds a vulnerability, the report shall include severity, reproduction steps, and suggested fix
  - Evidence: `run=go test -race -run Adversarial ./transport/ws/...; oracle=adversarial report verdict PASS, all [required] vectors covered; visibility=independent; controls=[agent,context]; missing=[]; companion=none`
  - Gates: `static`, `integration`
  - Executable doc: `transport/ws/adversarial_test.go`, `adversarial-report-ws.md`
  - Why not split vertically further?: WS server と client は同一フレームプロトコル・セッション管理を共有し、分断すると cross-cutting ベクター（セッション偽装、フレーム順序攻撃）を見落とす
  - Escalate if: Critical severity の脆弱性が設計レベルの変更を要求する場合

- [x] Theme: Adversarial Verify — Transport CLI (Phase 2/6)
  - Outcome: transport/cli の攻撃耐性が Critical tier で検証され、脆弱性が文書化される
  - Goal: CLI runner のフラグパース・JSON payload・stdin NDJSON ストリーミング・出力フォーマット・exitcode マッピングの攻撃耐性を検証
  - Target: transport/cli/runner.go, flag.go, json.go, format.go, stream.go, help.go, schema.go, node.go, exitcode.go
  - Attack Categories: 1 (Input Boundary), 2 (Error Handling), 3 (Security Boundary)
  - Must Not Break: 実装コード、既存テスト、attack-vectors.md
  - Non-goals: 脆弱性の修正 (別タスク)、generated files の直接検査
  - Acceptance (EARS):
    - When adversarial verification is executed at Critical tier, all [required] vectors within selected categories shall be probed or documented as N/A
    - When all probes result in DEFENDED and coverage gate passes, overall verdict shall be PASS
    - When a probe finds a vulnerability, the report shall include severity, reproduction steps, and suggested fix
  - Evidence: `run=go test -race -run Adversarial ./transport/cli/...; oracle=adversarial report verdict PASS, all [required] vectors covered; visibility=independent; controls=[agent,context]; missing=[]; companion=none`
  - Gates: `static`, `integration`
  - Executable doc: `transport/cli/adversarial_test.go`, `adversarial-report-cli.md`
  - Why not split vertically further?: flag パース・JSON ハンドリング・stream 入力は同一の runner dispatch パスを共有し、攻撃ベクターが相互に影響する
  - Escalate if: Critical severity の脆弱性が設計レベルの変更を要求する場合

- [x] Theme: Adversarial Verify — Config System (Phase 3/6)
  - Outcome: config パッケージの攻撃耐性が Critical tier で検証され、脆弱性が文書化される
  - Goal: 設定ロード (env/file/bootstrap)・バリデーション・シークレット redact・複合 config 衝突検出の攻撃耐性を検証
  - Target: config/load.go, env.go, file.go, parse.go, bootstrap.go, spec.go, required.go, redact.go
  - Attack Categories: 1 (Input Boundary), 2 (Error Handling), 3 (Security Boundary), 5 (State & Data Integrity)
  - Must Not Break: 実装コード、既存テスト、attack-vectors.md
  - Non-goals: 脆弱性の修正 (別タスク)、generated files の直接検査
  - Acceptance (EARS):
    - When adversarial verification is executed at Critical tier, all [required] vectors within selected categories shall be probed or documented as N/A
    - When all probes result in DEFENDED and coverage gate passes, overall verdict shall be PASS
    - When a probe finds a vulnerability, the report shall include severity, reproduction steps, and suggested fix
  - Evidence: `run=go test -race -run Adversarial ./config/...; oracle=adversarial report verdict PASS, all [required] vectors covered; visibility=independent; controls=[agent,context]; missing=[]; companion=none`
  - Gates: `static`, `integration`
  - Executable doc: `config/adversarial_test.go`, `adversarial-report-config.md`
  - Why not split vertically further?: env/file/bootstrap は同一の Load パイプラインを構成し、攻撃ベクター（型強制、デフォルト上書き、衝突）が複数ステージにまたがる
  - Escalate if: Critical severity の脆弱性が設計レベルの変更を要求する場合

- [x] Theme: Adversarial Verify — Transport Connect (Phase 4/6)
  - Outcome: transport/connect の攻撃耐性が Critical tier で検証され、脆弱性が文書化される
  - Goal: Connect ハンドラアダプタ (4 shape) ・エラーコードマッピング・ErrorMapper の攻撃耐性を検証
  - Target: transport/connect/connect.go, errors.go
  - Attack Categories: 1 (Input Boundary), 2 (Error Handling), 3 (Security Boundary), 4 (Concurrency)
  - Must Not Break: 実装コード、既存テスト、attack-vectors.md
  - Non-goals: 脆弱性の修正 (別タスク)、generated files の直接検査
  - Acceptance (EARS):
    - When adversarial verification is executed at Critical tier, all [required] vectors within selected categories shall be probed or documented as N/A
    - When all probes result in DEFENDED and coverage gate passes, overall verdict shall be PASS
    - When a probe finds a vulnerability, the report shall include severity, reproduction steps, and suggested fix
  - Evidence: `run=go test -race -run Adversarial ./transport/connect/...; oracle=adversarial report verdict PASS, all [required] vectors covered; visibility=independent; controls=[agent,context]; missing=[]; companion=none`
  - Gates: `static`, `integration`
  - Executable doc: `transport/connect/adversarial_test.go`, `adversarial-report-connect.md`
  - Why not split vertically further?: connect.go と errors.go は 2 ファイルのみで、handler adapter と error mapping は同一リクエストパスの入口と出口
  - Escalate if: Critical severity の脆弱性が設計レベルの変更を要求する場合

- [x] Theme: Adversarial Verify — Core Runtime (Phase 5/6)
  - Outcome: procframe ルートパッケージの攻撃耐性が Critical tier で検証され、脆弱性が文書化される
  - Goal: エラーモデル・インターセプターチェーン・Conn アダプタ・Request/Response/Meta の攻撃耐性を検証
  - Target: errors.go, interceptor.go, handler.go, stream.go, request.go, response.go, meta.go
  - Attack Categories: 1 (Input Boundary), 2 (Error Handling), 4 (Concurrency)
  - Must Not Break: 実装コード、既存テスト、attack-vectors.md
  - Non-goals: 脆弱性の修正 (別タスク)、generated files の直接検査
  - Acceptance (EARS):
    - When adversarial verification is executed at Critical tier, all [required] vectors within selected categories shall be probed or documented as N/A
    - When all probes result in DEFENDED and coverage gate passes, overall verdict shall be PASS
    - When a probe finds a vulnerability, the report shall include severity, reproduction steps, and suggested fix
  - Evidence: `run=go test -race -run Adversarial ./...; oracle=adversarial report verdict PASS, all [required] vectors covered; visibility=independent; controls=[agent,context]; missing=[]; companion=none`
  - Gates: `static`, `integration`
  - Executable doc: `adversarial_test.go`, `adversarial-report-core.md`
  - Why not split vertically further?: errors/interceptor/handler/stream は同一の handler 呼び出しパスを構成し、interceptor 内での error 伝播や Conn の Send/Receive が cross-cutting に関わる
  - Escalate if: Critical severity の脆弱性が設計レベルの変更を要求する場合

- [x] Theme: Adversarial Verify — Codegen (Phase 6/6)
  - Outcome: internal/codegen の攻撃耐性が Critical tier で検証され、脆弱性が文書化される
  - Goal: プラグインパラメータ処理・proto options 抽出・バリデーション (名前衝突、パス重複)・コード生成出力の正確性の攻撃耐性を検証
  - Target: internal/codegen/gen.go, handler.go, cli.go, connect.go, connect_client.go, ws.go, ws_client.go, config.go, config_model.go, tree.go, options.go, params.go, naming.go, validate.go
  - Attack Categories: 1 (Input Boundary), 2 (Error Handling), 5 (State & Data Integrity)
  - Must Not Break: 実装コード、既存テスト、attack-vectors.md
  - Non-goals: 脆弱性の修正 (別タスク)、generated files の直接検査
  - Acceptance (EARS):
    - When adversarial verification is executed at Critical tier, all [required] vectors within selected categories shall be probed or documented as N/A
    - When all probes result in DEFENDED and coverage gate passes, overall verdict shall be PASS
    - When a probe finds a vulnerability, the report shall include severity, reproduction steps, and suggested fix
  - Evidence: `run=go test -race -run Adversarial ./internal/codegen/...; oracle=adversarial report verdict PASS, all [required] vectors covered; visibility=independent; controls=[agent,context]; missing=[]; companion=none`
  - Gates: `static`, `integration`
  - Executable doc: `internal/codegen/adversarial_test.go`, `adversarial-report-codegen.md`
  - Why not split vertically further?: 14 ファイルだが全て同一の Generate() パイプラインを構成し、proto input → validation → code emission の一連のフローで攻撃ベクターが伝播する
  - Escalate if: Critical severity の脆弱性が設計レベルの変更を要求する場合
