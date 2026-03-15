# procframe v0.1 設計書

## 1. 定義

`procframe` は、**proto を利用者側で定義し、こちらが plugin と runtime を提供する**小さな Go ライブラリである。

目的は次の 2 つを、**最小の抽象**でつなぐこと。

```text
1. env / cli / file -> merge -> immutable config
2. CLI / WS -> typed request -> handler -> typed response
```

v0.1 では、**個人利用・単独ライブラリ・シンプルさ優先**を前提に、対象を次へ限定する。

* transport:

  * CLI
  * generic WebSocket(JSON over text frames)
* handler shape:

  * unary
  * server-stream
* schema:

  * protobuf
* codegen:

  * `protoc-gen-procframe-go`
* config file:

  * JSON のみ

HTTP / gRPC は **未実装**だが、将来追加しても破綻しないように、procedure 名・meta・error code・stream 抽象だけは最初から揃えておく。

---

## 2. 背景と狙い

今回の設計で解決したい課題は次の通り。

* `kong.Bind()` 的な **CLI parser / DI / business logic の癒着**を避けたい
* 同一バイナリで **CLI と Discord 系 WS 利用**を成立させたい
* ただしライブラリ自体は **Discord 専用に寄せたくない**
* request/response の schema は **proto を single source of truth** にしたい
* config も transport も **codegen で boilerplate を消したい**
* ただし最初から機能を盛り込みすぎたくない

そのため、v0.1 は次の思想で固定する。

```text
proto schema
  -> generated config loader
  -> generated CLI glue
  -> generated WS glue
  -> handwritten handler
```

---

## 3. スコープ

## 3.1 v0.1 でやること

* `options.proto` をライブラリから提供する
* 利用者は `config.proto` と `service proto` を書く
* plugin が次を生成する

  * config loader
  * CLI command tree / parser / dispatch
  * WS dispatch / frame codec / stream glue
  * handler interface
* runtime は最小限の typed abstraction を持つ
* service は CLI group として扱える
* group は **Run を持たず flags だけ持てる**
* method ごとに CLI / WS 公開可否を制御できる
* 未指定時の公開ポリシーは

  * `cli = true`
  * `ws = false`

## 3.2 v0.1 でやらないこと

* HTTP
* gRPC
* bidi stream
* client stream
* YAML / TOML config
* Discord transport の内蔵
* middleware/interceptor フレームワーク
* reflection ベースの汎用 registry
* option の複数記法
* CLI/WS 以外の transport codegen

---

## 4. 設計原則

## 4.1 runtime は小さく、賢さは plugin に寄せる

runtime は transport 非依存の最小核だけを持つ。
CLI tree 構築、flag parser、WS dispatch などは codegen で静的に作る。

## 4.2 proto は schema、runtime format ではない

proto は config / procedure の **定義**に使う。
実際の入力は次の通り。

* config file: JSON
* env: 環境変数
* bootstrap CLI: 起動時設定
* procedure CLI: サブコマンド引数
* WS: JSON frame

## 4.3 service は CLI group、method は runnable procedure

service 自体を runnable にしない。
service は CLI tree の中間ノードとして使う。
WS では service は namespace にすぎず、**method だけが procedure** である。

## 4.4 transport ごとの公開は method 単位で制御する

v0.1 のデフォルトはこれ。

```text
cli = true
ws  = false
```

何も書かない RPC は CLI だけに出る。
WS に出したいものだけ `ws: true` を付ける。

## 4.5 group flags は canonical request への sugar として扱う

`app repo --org my pr list` の `--org` は、CLI 専用の裏値ではなく、最終 request に詰まる値でなければならない。
CLI と WS で request schema が分裂しないようにする。

---

## 5. 全体アーキテクチャ

```text
user proto
├─ config.proto
├─ service.proto
└─ import procframe/options/v1/options.proto
        ↓
protoc
├─ protoc-gen-go
└─ protoc-gen-procframe-go
        ↓
generated code
├─ *.pb.go
├─ *.config.proc.go
├─ *.cli.proc.go
└─ *.ws.proc.go
        ↓
app code
├─ implements generated handler interface
└─ chooses transport runner
        ↓
runtime
├─ Request[T]
├─ Response[T]
├─ ServerStream[T]
└─ error/meta helpers
```

---

## 6. ライブラリ構成

```text
procframe/
  request.go
  response.go
  stream.go
  meta.go
  errors.go

  config/
    jsonfile.go
    merge.go
    bootargs.go

  transport/
    cli/
      runner.go
      help.go
    ws/
      handler.go
      frame.go
      stream.go

  proto/
    procframe/options/v1/options.proto

  cmd/
    protoc-gen-procframe-go/
      main.go
```

### 各要素の責務

* `procframe/*`

  * typed runtime abstraction
* `procframe/config/*`

  * generated config loader が使う共通 helper
* `procframe/transport/cli/*`

  * generated CLI tree を実行する小さな runner
* `procframe/transport/ws/*`

  * generated WS dispatch を実行する小さな handler
* `proto/procframe/options/v1/options.proto`

  * ライブラリ利用者に公開する schema API
* `protoc-gen-procframe-go`

  * glue code 生成

---

## 7. runtime 抽象

## 7.1 request / response

```go
type Meta struct {
	Procedure string
	RequestID string
	SessionID string
	Labels    map[string]string
}

type Request[T any] struct {
	Msg  *T
	Meta Meta
}

type Response[T any] struct {
	Msg  *T
	Meta Meta
}
```

### 目的

* transport 依存型を handler に漏らさない
* 将来 HTTP/gRPC を足すときの metadata 置き場を確保する
* procedure 名を一貫して保持する

## 7.2 handler

```go
type UnaryHandler[Req, Res any] interface {
	Handle(context.Context, *Request[Req]) (*Response[Res], error)
}

type ServerStream[Res any] interface {
	Context() context.Context
	Send(*Response[Res]) error
}

type ServerStreamHandler[Req, Res any] interface {
	HandleStream(context.Context, *Request[Req], ServerStream[Res]) error
}
```

### v0.1 の正式サポート

* unary
* server-stream

client-stream / bidi は入れない。

## 7.3 error

```go
type Code string

const (
	CodeInvalidArgument Code = "invalid_argument"
	CodeNotFound        Code = "not_found"
	CodeInternal        Code = "internal"
	CodeUnauthenticated Code = "unauthenticated"
	CodeUnavailable     Code = "unavailable"
)

type Error struct {
	Code    Code
	Message string
	Cause   error
}
```

### 目的

* CLI: stderr / exit code へ変換
* WS: error frame へ変換
* 将来 HTTP/gRPC 追加時の mapping を安定させる

---

## 8. config 設計

## 8.1 config の流れ

```text
argv
├─ bootstrap flags
└─ procedure args
```

config 解決の流れは固定する。

```text
defaults
  -> file(JSON)
  -> env
  -> bootstrap CLI
  -> validate
  -> immutable config
```

## 8.2 file format

v0.1 は **JSON のみ**。

理由:

* protojson と素直につながる
* 依存が少ない
* 実装が小さい

## 8.3 generated API

`config.proto` から例えば次を生成する。

```go
func LoadRuntimeConfig(argv []string) (*RuntimeConfig, []string, error)
```

戻り値:

* `*RuntimeConfig`: immutable config
* `[]string`: procedure 用に残した argv
* `error`

## 8.4 bootstrap CLI

bootstrap CLI は config 用にだけ使う。
例:

```bash
app --config config.json --log-level debug repo pr list --limit 20
```

ここで `--config` と `--log-level` は bootstrap。
`repo pr list --limit 20` は procedure args。

---

## 9. CLI / WS の抽象化

## 9.1 CLI

CLI は human-facing transport。
再帰的サブコマンド tree を持つ。

### 役割

* command path 解決
* group flags の収集
* leaf request flags の parse
* request message の構築
* handler 呼び出し
* output render

### サポート形

* unary -> stdout
* server-stream -> chunk ごとに stdout

## 9.2 WS

WS は machine-facing transport。
route は **canonical procedure 名**のみ。

### 役割

* inbound frame decode
* procedure 名による static dispatch
* request payload unmarshal
* handler 呼び出し
* outbound frame encode

### サポート形

* unary -> 1 frame + `eos=true`
* server-stream -> 複数 frame

### 重要なルール

WS では **service root を route にしない**。
有効なのは method だけ。

```text
valid:   /app.bot.v1.RepoPRService/List
invalid: /app.bot.v1.RepoPRService/
```

---

## 10. `options.proto` 設計

これは **ライブラリ提供物**であり、利用者は import して使う。

```proto
import "procframe/options/v1/options.proto";
```

v0.1 の公開 DSL は、**1 つの書き方に絞る**。

* route 記法は `path` だけ
* `prefix` / `name` / full-path override は持たない
* group bind は `bind_into` だけ
* type 名は option に書かない

---

## 11. `options.proto` の v0.1 仕様

```proto
syntax = "proto2";

package procframe.options.v1;
option go_package = "github.com/you/procframe/proto/procframe/options/v1;procframeoptionsv1";

import "google/protobuf/descriptor.proto";

message CliPath {
  repeated string segments = 1;
}

message ConfigFieldOptions {
  optional string env = 1;
  optional string default_string = 2;
  optional bool required = 3 [default = false];
  optional bool secret = 4 [default = false];
  optional bool bootstrap = 5 [default = false];
}

message CliGroupOptions {
  optional CliPath path = 1;
  optional string bind_into = 2; // request の top-level message field 名
  optional string summary = 3;
  optional bool hidden = 4 [default = false];
}

message ProcOptions {
  optional CliPath cli_path = 1; // service path に対する相対
  optional bool cli = 2 [default = true];
  optional bool ws = 3 [default = false];
  optional string summary = 4;
  optional bool hidden = 5 [default = false];
}

extend google.protobuf.FieldOptions {
  optional ConfigFieldOptions config = 51001 [retention = RETENTION_SOURCE];
}

extend google.protobuf.ServiceOptions {
  optional CliGroupOptions cli_group = 52001 [retention = RETENTION_SOURCE];
}

extend google.protobuf.MethodOptions {
  optional ProcOptions proc = 52002 [retention = RETENTION_SOURCE];
}
```

---

## 12. CLI DSL の意味

## 12.1 service は group

```proto
service RepoService {
  option (procframe.options.v1.cli_group) = {
    path: { segments: "repo" }
    bind_into: "repo"
  };
}
```

意味:

* CLI tree 上の `repo` グループ
* runnable ではない
* descendant RPC の request にある `repo` field へ group flags を bind する

## 12.2 method は leaf

```proto
rpc List(PullRequestListRequest) returns (PullRequestListResponse) {
  option (procframe.options.v1.proc) = {
    cli_path: { segments: "list" }
  };
}
```

意味:

* group path の下の `list` leaf
* CLI では公開
* WS では非公開（未指定なので `false`）

## 12.3 route の解決

effective CLI path はこう決まる。

```text
effective path = service.cli_group.path + method.proc.cli_path
```

例:

```text
service path = ["repo", "pr"]
method path  = ["list"]
effective    = ["repo", "pr", "list"]
```

---

## 13. group flags の bind 仕様

## 13.1 `bind_into` の意味

`bind_into: "repo"` は **field 名**である。
型名ではない。

つまり plugin は leaf request descriptor を見て、

```proto
message PullRequestListRequest {
  RepoScope repo = 1;
  PRScope pr = 2;
  int32 limit = 3;
}
```

この `repo` field の型が `RepoScope` だと判定する。

### 明確なルール

* `bind_into` は request の **top-level field 名**
* 命名規則から型名を推定しない
* 型は **field descriptor から取得**する

## 13.2 group flags から request への注入

たとえば:

```bash
app repo --org my pr --state open list --limit 20
```

最終 request はこうなる。

```json
{
  "repo": { "org": "my" },
  "pr":   { "state": "open" },
  "limit": 20
}
```

### 意味

* `repo` group flags -> request.repo
* `pr` group flags -> request.pr
* leaf flags -> request.limit

CLI 専用の隠し状態は持たない。

## 13.3 plugin の検証

group `bind_into` について、plugin は **CLI 公開される descendant method だけ**を見る。

検証内容:

* request に対象 field が存在する
* その field が message field である
* 同一 group 配下の全 descendant request で型が一致する

不一致なら codegen error。

---

## 14. transport 公開ポリシー

## 14.1 デフォルト

```text
cli = true
ws  = false
```

## 14.2 method ごとの制御

```proto
rpc Watch(PullRequestWatchRequest) returns (stream PullRequestWatchChunk) {
  option (procframe.options.v1.proc) = {
    cli_path: { segments: "watch" }
    ws: true
  };
}
```

この場合だけ WS dispatch に乗る。

## 14.3 service root の扱い

service root はどの transport でも runnable にしない。

* CLI では tree node
* WS では namespace
* `"/package.Service/"` は常に無効

## 14.4 dead group prune

CLI tree 生成時、

* `cli_group` があっても
* 配下に `cli=true` な leaf method が 1 つもなければ
* その group は生成しない

WS 側にはそもそも group がない。

---

## 15. codegen の生成物

service proto から同一 Go package に次を生成する。

```text
foo.proto
├─ foo.pb.go
├─ foo.cli.proc.go
├─ foo.ws.proc.go
└─ foo.handler.proc.go
```

config proto からは

```text
config.proto
├─ config.pb.go
└─ config.proc.go
```

## 15.1 handler interface

```go
type BotServiceHandler interface {
	Ping(
		context.Context,
		*procframe.Request[PingRequest],
	) (*procframe.Response[PingResponse], error)

	Chat(
		context.Context,
		*procframe.Request[ChatRequest],
		procframe.ServerStream[ChatChunk],
	) error
}
```

## 15.2 CLI runner

```go
func NewBotServiceCLIRunner(h BotServiceHandler) *cli.Runner
```

役割:

* static command tree を持つ
* leaf ごとの `flag.FlagSet` parser を使う
* group flags を request に bind する

## 15.3 WS handler

```go
func NewBotServiceWSHandler(h BotServiceHandler) *ws.Handler
```

役割:

* procedure 名で static dispatch
* unary / stream を JSON frame に変換

---

## 16. CLI 実装方針

## 16.1 parser

Kong は使わない。
generated `flag.FlagSet` を method / group ごとに生成する。

### 理由

* 依存が少ない
* bind 問題が消える
* codegen と相性が良い

## 16.2 対応フィールド

v0.1 では次を優先する。

* string
* bool
* int32 / int64
* uint32 / uint64
* float
* enum

nested message は、group bind 対象以外ではまず限定対応にする。
必要なら leaf では `--json` fallback を許す。

## 16.3 command tree

generated static tree を使う。

```go
type commandNode struct {
	Segment  string
	Children map[string]*commandNode
	Leaf     func(context.Context, []string) error
	Summary  string
	Hidden   bool
}
```

`Run()` は tree を辿るだけ。

---

## 17. WS 実装方針

## 17.1 frame 形式

v0.1 は JSON over text frames。

### inbound

```json
{
  "id": "req-1",
  "procedure": "/app.bot.v1.BotService/Ping",
  "payload": {
    "target": "local"
  }
}
```

### outbound unary

```json
{
  "id": "req-1",
  "payload": {
    "message": "pong: local"
  },
  "eos": true
}
```

### outbound stream

```json
{"id":"req-1","payload":{"text":"he"},"eos":false}
{"id":"req-1","payload":{"text":"llo"},"eos":false}
{"id":"req-1","payload":{"done":true},"eos":true}
```

### outbound error

```json
{
  "id": "req-1",
  "error": {
    "code": "invalid_argument",
    "message": "..."
  },
  "eos": true
}
```

## 17.2 dispatch

WS dispatch は **method だけ**を持つ static map。

```go
switch env.Procedure {
case "/app.bot.v1.BotService/Ping":
	...
case "/app.bot.v1.BotService/Chat":
	...
default:
	...
}
```

service root は実装しない。

---

## 18. Discord との関係

`procframe` の v0.1 core は **generic WS** だけを扱う。
Discord Gateway は内蔵しない。

理由:

* Discord は generic WS ではない
* Gateway input + REST output の特殊 adapter である
* core を Discord 色に染めたくない

### 位置づけ

```text
procframe core:
  CLI + generic WS

optional app-side bridge:
  Discord Gateway event
    -> request
    -> handler
    -> Discord REST send
```

この bridge は app 側、または将来 optional addon として外出しする。

---

## 19. 将来 HTTP/gRPC を足すときの前提

今は実装しないが、次の制約は守る。

### 19.1 procedure 名は method full name ベース

```text
/package.Service/Method
```

### 19.2 handler は transport 非依存

handler は次を知らない。

* `[]string`
* `websocket.Conn`
* `discordgo.*`
* `http.Request`

### 19.3 meta を持つ

少なくとも以下を持つ。

* `Procedure`
* `RequestID`
* `SessionID`

### 19.4 error は code 付き

### 19.5 unary と server-stream を分ける

これで HTTP / gRPC / Connect を将来足しても core は崩れない。

---

## 20. 比較

## 20.1 ルーティング DSL を複数持つ案

例:

* `prefix`
* `name`
* full path override

### 問題

* 書き方が複数あり混乱する
* service/method で概念がずれる
* group flags を足すとさらに複雑になる

## 20.2 `path` に一本化した案

### 長所

* service / method で同じ概念
* route が 1 記法に揃う
* 再帰サブコマンドに自然

**採用**

---

## 20.3 `bind_into + type string` 案

例:

```proto
bind_into: "repo"
message: ".app.bot.v1.RepoScope"
```

### 問題

* 冗長
* rename に弱い
* `bind_into` から request descriptor を見れば十分

## 20.4 `bind_into` だけにする案

### 長所

* DSL が小さい
* 型は descriptor から確定できる
* compile-time 検証できる

**採用**

---

## 20.5 `cli/ws` とも default true の案

### 問題

* WS に意図せず公開される
* 将来 transport を増やすと危険

## 20.6 `cli=true, ws=false` の案

### 長所

* 安全
* わかりやすい
* 将来 HTTP/gRPC 追加とも相性が良い

**採用**

---

## 21. 具体例

## 21.1 `options.proto`

```proto
syntax = "proto2";

package procframe.options.v1;
option go_package = "github.com/you/procframe/proto/procframe/options/v1;procframeoptionsv1";

import "google/protobuf/descriptor.proto";

message CliPath {
  repeated string segments = 1;
}

message ConfigFieldOptions {
  optional string env = 1;
  optional string default_string = 2;
  optional bool required = 3 [default = false];
  optional bool secret = 4 [default = false];
  optional bool bootstrap = 5 [default = false];
}

message CliGroupOptions {
  optional CliPath path = 1;
  optional string bind_into = 2;
  optional string summary = 3;
  optional bool hidden = 4 [default = false];
}

message ProcOptions {
  optional CliPath cli_path = 1;
  optional bool cli = 2 [default = true];
  optional bool ws = 3 [default = false];
  optional string summary = 4;
  optional bool hidden = 5 [default = false];
}

extend google.protobuf.FieldOptions {
  optional ConfigFieldOptions config = 51001 [retention = RETENTION_SOURCE];
}

extend google.protobuf.ServiceOptions {
  optional CliGroupOptions cli_group = 52001 [retention = RETENTION_SOURCE];
}

extend google.protobuf.MethodOptions {
  optional ProcOptions proc = 52002 [retention = RETENTION_SOURCE];
}
```

---

## 21.2 config proto

```proto
syntax = "proto3";

package app.config.v1;
option go_package = "example/gen/app/config/v1;configv1";

import "procframe/options/v1/options.proto";

message RuntimeConfig {
  string log_level = 1 [
    (procframe.options.v1.config).env = "LOG_LEVEL",
    (procframe.options.v1.config).default_string = "info",
    (procframe.options.v1.config).bootstrap = true
  ];

  string ws_listen_addr = 2 [
    (procframe.options.v1.config).env = "WS_LISTEN_ADDR",
    (procframe.options.v1.config).default_string = ":8080",
    (procframe.options.v1.config).bootstrap = true
  ];

  string discord_token = 3 [
    (procframe.options.v1.config).env = "DISCORD_TOKEN",
    (procframe.options.v1.config).required = true,
    (procframe.options.v1.config).secret = true
  ];
}
```

---

## 21.3 service proto

```proto
syntax = "proto3";

package app.bot.v1;
option go_package = "example/gen/app/bot/v1;botv1";

import "procframe/options/v1/options.proto";

message RepoScope {
  string org = 1;
}

message PRScope {
  string state = 1;
}

message PullRequestListRequest {
  RepoScope repo = 1;
  PRScope pr = 2;
  int32 limit = 3;
}

message PullRequestListResponse {
  repeated string items = 1;
}

message PullRequestWatchRequest {
  RepoScope repo = 1;
  PRScope pr = 2;
}

message PullRequestWatchChunk {
  string text = 1;
  bool done = 2;
}

service RepoService {
  option (procframe.options.v1.cli_group) = {
    path: { segments: "repo" }
    bind_into: "repo"
  };
}

service RepoPRService {
  option (procframe.options.v1.cli_group) = {
    path: { segments: "repo" segments: "pr" }
    bind_into: "pr"
  };

  rpc List(PullRequestListRequest) returns (PullRequestListResponse) {
    option (procframe.options.v1.proc) = {
      cli_path: { segments: "list" }
    };
  }

  rpc Watch(PullRequestWatchRequest) returns (stream PullRequestWatchChunk) {
    option (procframe.options.v1.proc) = {
      cli_path: { segments: "watch" }
      ws: true
    };
  }
}
```

---

## 21.4 CLI から見える形

```bash
app repo --org my pr --state open list --limit 20
app repo --org my pr --state open watch
```

* `list` は CLI のみ
* `watch` は CLI + WS

---

## 21.5 WS から見える形

有効:

```json
{
  "id": "req-1",
  "procedure": "/app.bot.v1.RepoPRService/Watch",
  "payload": {
    "repo": { "org": "my" },
    "pr":   { "state": "open" }
  }
}
```

無効:

```text
/app.bot.v1.RepoPRService/
/app.bot.v1.RepoPRService/List   // ws:false のため未公開
```

---

## 21.6 app 側の実装

```go
type RepoHandler struct{}

func (h *RepoHandler) List(
	ctx context.Context,
	req *procframe.Request[botv1.PullRequestListRequest],
) (*procframe.Response[botv1.PullRequestListResponse], error) {
	return &procframe.Response[botv1.PullRequestListResponse]{
		Msg: &botv1.PullRequestListResponse{
			Items: []string{"a", "b"},
		},
	}, nil
}

func (h *RepoHandler) Watch(
	ctx context.Context,
	req *procframe.Request[botv1.PullRequestWatchRequest],
	s procframe.ServerStream[botv1.PullRequestWatchChunk],
) error {
	for _, part := range []string{"he", "llo"} {
		if err := s.Send(&procframe.Response[botv1.PullRequestWatchChunk]{
			Msg: &botv1.PullRequestWatchChunk{Text: part},
		}); err != nil {
			return err
		}
	}
	return s.Send(&procframe.Response[botv1.PullRequestWatchChunk]{
		Msg: &botv1.PullRequestWatchChunk{Done: true},
	})
}
```

---

## 22. main のイメージ

```go
func main() {
	cfg, rest, err := configv1.LoadRuntimeConfig(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	h := &RepoHandler{}

	if len(rest) > 0 && rest[0] == "serve-ws" {
		wsHandler := botv1.NewRepoPRServiceWSHandler(h)
		if err := ws.ListenAndServe(cfg.WsListenAddr, wsHandler); err != nil {
			log.Fatal(err)
		}
		return
	}

	cliRunner := botv1.NewRepoPRServiceCLIRunner(h)
	if err := cliRunner.Run(context.Background(), rest); err != nil {
		log.Fatal(err)
	}
}
```

transport 起動の orchestration は app 側に残す。
ライブラリは glue までに留める。

---

## 23. 採用方針まとめ

### 採用

* 単独ライブラリ
* proto import 方式の `options.proto`
* `config.proto` と `service proto` を分ける
* config: `defaults -> file(JSON) -> env -> bootstrap CLI -> immutable`
* runtime は最小の typed abstraction
* CLI/WS glue は codegen
* CLI route は `path` の 1 記法のみ
* service は CLI group
* method は runnable leaf
* group flags は `bind_into` で request field に bind
* 型は request field descriptor から推定
* `cli=true` default
* `ws=false` default
* WS は method のみ公開
* service root は常に無効
* dead CLI group は prune
* Discord は core の外

### 不採用

* `prefix` / `name` / full-path override の併存
* bind type 名の string 指定
* Kong / reflection 中心設計
* YAML 初期対応
* Discord transport の内蔵
* default all transports true

---

## 24. 次に固定すべきもの

この設計の次段階で固定するべき仕様は 3 つ。

### 24.1 plugin 出力 API の正式名

* `NewXxxCLIRunner`
* `NewXxxWSHandler`
* `LoadRuntimeConfig`

### 24.2 CLI field-to-flag 変換規則

* snake_case / kebab-case
* enum 表現
* repeated の扱い
* `--json` fallback 範囲

### 24.3 WS frame の正式 schema

* inbound / outbound / error / eos
* unary / stream の表現
* request id の扱い

---

## 25. 結論

`procframe v0.1` は、次の一文に尽きる。

**「proto で config と procedure を定義し、plugin が CLI と WS の glue code を生成する、小さな typed runtime ライブラリ」**

この設計なら、

* 直近で必要な CLI + WS を最初から使える
* config merge も同じ思想で統一できる
* service/group と method/leaf の責務が明確
* future HTTP/gRPC を足しても core は崩れない
* それでいて v0.1 の実装量は必要最小限に抑えられる
