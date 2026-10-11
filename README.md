# tolo-edge-bridge

## 開発

### セットアップ

mise を使用して開発環境をセットアップする  
`mise install` の後処理で lefthook の pre-commit フック（シークレット検出と golangci-lint）が導入される

```bash
mise trust
mise install
task proto
```

### テストと lint

```bash
go build ./...
go test ./...
golangci-lint run
```

### Docker Compose での起動

Docker Compose で開発サーバーと Firestore エミュレータを起動できる

```bash
docker compose up --build
```

もしくは
```bash
task up:build
```

サーバーは `http://localhost:8080` で待ち受ける（停止は `task down`）  
`server` は `FIRESTORE_EMULATOR_HOST=firebase-emulator:8080` で compose 内の Firestore エミュレータ（プロジェクト `demo-tolo`）に接続する  
エミュレータは `firebase-emulator/` の Dockerfile からビルドし、ホストへは Firestore を `localhost:8081`、Emulator UI を `http://localhost:4000` で公開する。ホストの 8080 番はサーバーが使うので、Firestore は 8081 番にずらしている

### シナリオテスト（runn）

[runn](https://github.com/k1LoW/runn) の runbook `scripts/dev/runbooks/signaling.yml` で、compose で起動したサーバーとエミュレータを通しで確認する  
runn はホストに入れず、compose の `runn` サービス（profile `runn`）としてコンテナで動かす

```bash
task runn
```

`task runn` は compose を起動してから `docker compose run --rm -T runn` を実行する  
runbook はエミュレータのデータを消してから、`lastSeenAt` が現在時刻の edge と 60 秒前の edge を Firestore の REST API で置き、次を確かめる

- `ListAliveEdges` が現在時刻の edge だけを返す
- `RequestConnection` が session ID を返し、`edges/{edgeId}/sessions/{sessionId}` に `status: "requested"` と `createdAt` が書き込まれる
- 空の `edge_id` は `invalid_argument`、存在しない edge は `not_found` になる

### 環境変数

| 環境変数 | デフォルト | 内容 |
| --- | --- | --- |
| `SERVER_ADDR` | `:8080` | 待ち受けるアドレス |
| `FIRESTORE_PROJECT_ID` | なし（必須） | 接続する Firestore の GCP プロジェクト ID。未設定ならサーバーは起動しない |
| `FIRESTORE_EMULATOR_HOST` | なし | 設定すると Firestore クライアントが本番ではなくそのホストのエミュレータに接続する（クライアントライブラリの標準の挙動）。本番では設定しない |
| `LOG_LEVEL` | `info` | ログに出力する最小レベル。`debug`／`info`／`warn`（`warning` も同義）／`error`／`critical` を取り、未知の値ならサーバーは起動しない |
| `GOOGLE_CLOUD_PROJECT` | なし | 設定するとログの `logging.googleapis.com/trace` を `projects/<project>/traces/<trace_id>` 形式にし、Cloud Logging でトレースと相関させる。未設定なら素のトレース ID を出力する |
| `OTEL_EXPORTER_OTLP_ENDPOINT` / `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | なし | どちらかを設定したときだけ OTLP/HTTP で span を export する。未設定ならトレーシングは無効 |
| `OTEL_SERVICE_NAME` | `tolo-edge-bridge` | トレースの `service.name` |

ヘッダ・TLS・タイムアウトなどその他の `OTEL_EXPORTER_OTLP_*` は exporter がそのまま解釈する  
ログは標準出力へ 1 行 1 件の JSON で書き出す  
Firestore の認証はクライアントライブラリの Application Default Credentials に従う

### トレースの確認（Jaeger）

監視スタックはオーバーライドファイル `compose.o11y.yml` を重ねたときだけ有効になる  
Jaeger が起動し、`server` に OTLP エクスポート用の環境変数（`OTEL_EXPORTER_OTLP_ENDPOINT` など）がセットされる

```bash
docker compose -f compose.yml -f compose.o11y.yml up --build
```

もしくは
```bash
task up:build:o11y
```

Jaeger UI は `http://localhost:16686`（停止は `task down:o11y`）

### connect-es の生成

connect-es の生成（`task proto:gen:es`）はリリース時に CI で行う  
ローカルで実行する場合は `clients/connect-es` の依存（`npm i`）を導入する必要がある

## proto アーティファクトの利用

`.proto` は [ORAS](https://oras.land) で OCI アーティファクト化され、GitHub Container Registry に公開される  
アーティファクト名: `ghcr.io/<owner>/<repo>-proto`

### 取得（pull）

[ORAS CLI](https://oras.land/docs/installation) が必要（mise でも導入される）

```bash
# 出力先ディレクトリに proto を展開（ディレクトリ構造が復元される）
oras pull ghcr.io/<owner>/<repo>-proto:latest -o proto
```

取得した `.proto` は `buf` や `protoc` の入力としてそのまま利用できる
