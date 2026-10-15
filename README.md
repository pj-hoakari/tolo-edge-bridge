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
`fakeidp`（`http://localhost:8082`）は tolo-service-gateway の開発専用の偽 IdP で、`POST /token` の本文をそのまま claim にした `event_access` を検証なしで発行する。compose は tolo-service-gateway の git リポジトリをビルドコンテキストにして、コミットを固定した `fakeidp` ターゲットをビルドする  
`server` は `IDP_JWKS_URL=http://fakeidp:8080/oauth2/jwks` でこの偽 IdP の鍵を参照し、`console-ui` を console 役、`edge-page` を edge 役の client として受理する  
`server` は `FIREBASE_AUTH_EMULATOR_HOST=firebase-emulator:9099` で compose 内の Firebase Auth エミュレータを使い、カスタムトークンを署名なしで発行する  
エミュレータは `firebase-emulator/` の Dockerfile からビルドし、ホストへは Firestore を `localhost:8081`、Firebase Auth を `localhost:9099`、Emulator UI を `http://localhost:4000` で公開する。ホストの 8080 番はサーバーが使うので、Firestore は 8081 番にずらしている  
シグナリング文書のセキュリティルールは `firebase-emulator/firestore.rules` にあり、同じディレクトリの `firebase.json` が参照する。Firestore エミュレータはこのルールを読み込んで適用するので、`Authorization: Bearer owner`（Admin SDK 相当）以外の要求はルールに従う

### シナリオテスト（runn）

[runn](https://github.com/k1LoW/runn) の runbook `scripts/dev/runbooks/signaling.yml` で、compose で起動したサーバーとエミュレータを通しで確認する  
runn はホストに入れず、compose の `runn` サービス（profile `runn`）としてコンテナで動かす

```bash
task runn
```

`task runn` は compose を起動してから `docker compose run --rm -T runn` を実行する  
runbook はエミュレータのデータを消し、`fakeidp` からトークンを得てから、次を確かめる

- console 役の `event_access` を付けた `RequestConnection` が session ID を返し、トークンの `tenant_id` と `event_id` に従う `tenants/{tenantId}/events/{eventId}/edgeDevices/{edgeDeviceId}/sessions/{sessionId}` に `status: "requested"` と `createdAt` が書き込まれる
- トークンがない要求は `unauthenticated` になる
- edge 役のトークンでの `RequestConnection` は `permission_denied` になる
- 受理しない `client_id` のトークンは `permission_denied` になる
- 空の `edge_id` は `invalid_argument` になる
- edge 役と console 役の `event_access` を付けた `IssueFirestoreToken` がカスタムトークンを返し、Auth エミュレータの `signInWithCustomToken` でサインインできる。サインインしたユーザーの uid はトークンの `sub` で、カスタムクレームに `tenant_id`、`event_id`、`peer_role`、`expires_at`、`token_use`（`signaling`）を持ち、`expires_at` はレスポンスの `expires_at` と同じ時刻を指す
- トークンがない `IssueFirestoreToken` は `unauthenticated`、受理しない `client_id` のトークンは `permission_denied` になる
- サインインで得た ID トークンを Firestore エミュレータの REST API に付けて、セキュリティルールを確かめる。edge 役と console 役は自分のセッションと signals を読め、signals は `from` が自分の役割で、フィールドが `from`、`payload`、`createdAt` だけで、`createdAt` がサーバー時刻（`REQUEST_TIME`）のときだけ作れる。`from` の不一致、余分なフィールド、クライアントが決めた `createdAt`、signals の更新、セッションの作成と更新、edge 役による削除、別の tenant や event のパス、トークンなし、`expires_at` を過ぎたトークン、`token_use` が `signaling` でないトークン、コレクショングループクエリは拒否され、console 役はセッションと signals を削除できる
- `expires_at` を過ぎたトークンはサーバーが発行しないので、runbook が edge 役の ID トークンの claim を写して `expires_at` を過去にした署名なし（`alg: none`）の JWT を作り、エミュレータに渡す。`token_use` が別の値のトークンも同じ方法で作る

### 環境変数

| 環境変数 | デフォルト | 内容 |
| --- | --- | --- |
| `SERVER_ADDR` | `:8080` | 待ち受けるアドレス |
| `FIRESTORE_PROJECT_ID` | なし（必須） | 接続する Firestore の GCP プロジェクト ID。Firebase Auth のカスタムトークンも同じプロジェクトで発行する。未設定ならサーバーは起動しない |
| `FIRESTORE_EMULATOR_HOST` | なし | 設定すると Firestore クライアントが本番ではなくそのホストのエミュレータに接続する（クライアントライブラリの標準の挙動）。本番では設定しない |
| `FIREBASE_AUTH_EMULATOR_HOST` | なし | 設定すると Firebase Admin SDK がそのホストの Auth エミュレータ向けに、署名なし（`alg: none`）のカスタムトークンを発行する（SDK の標準の挙動）。本番では設定しない |
| `IDP_ISSUER` | なし（必須） | `event_access` に要求する `iss`。未設定ならサーバーは起動しない |
| `IDP_AUDIENCE` | なし（必須） | `event_access` に要求する `aud`（バックエンド API 全体の論理 audience。例 `backend-api`。Service Gateway と同じ値） |
| `IDP_JWKS_URL` | なし（必須） | IdP の JWKS の絶対 HTTP URL。鍵は 5 分キャッシュし、未知の `kid` は 30 秒に 1 回まで即時に再取得する |
| `CONSOLE_CLIENT_ID` | なし（必須） | console 役（管理 UI）として受理する `client_id` |
| `EDGE_CLIENT_ID` | なし（必須） | edge 役（観測ページ）として受理する `client_id`。`CONSOLE_CLIENT_ID` と同じ値は設定エラーになる |
| `LOG_LEVEL` | `info` | ログに出力する最小レベル。`debug`／`info`／`warn`（`warning` も同義）／`error`／`critical` を取り、未知の値ならサーバーは起動しない |
| `GOOGLE_CLOUD_PROJECT` | なし | 設定するとログの `logging.googleapis.com/trace` を `projects/<project>/traces/<trace_id>` 形式にし、Cloud Logging でトレースと相関させる。未設定なら素のトレース ID を出力する |
| `OTEL_EXPORTER_OTLP_ENDPOINT` / `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | なし | どちらかを設定したときだけ OTLP/HTTP で span を export する。未設定ならトレーシングは無効 |
| `OTEL_SERVICE_NAME` | `tolo-edge-bridge` | トレースの `service.name` |

ヘッダ・TLS・タイムアウトなどその他の `OTEL_EXPORTER_OTLP_*` は exporter がそのまま解釈する  
ログは標準出力へ 1 行 1 件の JSON で書き出す  
Firestore の認証はクライアントライブラリの Application Default Credentials に従う  
カスタムトークンの署名も Application Default Credentials に従う。サービスアカウントの鍵ファイルがない環境（Cloud Run など）では Admin SDK が IAM の `signBlob` で署名するため、実行サービスアカウントに `roles/iam.serviceAccountTokenCreator` が必要になる。認証情報がなければサーバーは起動しない  
全 RPC は本文を読む前に `Authorization: Bearer` の `event_access` を検証し、`client_id` から役割を決める。`RequestConnection` は console 役だけが呼べる。`IssueFirestoreToken` は edge 役と console 役が呼べ、`event_access` の `sub` を uid、`tenant_id`、`event_id`、役割（`peer_role`）、`exp`（`expires_at`、UNIX 秒）、用途（`token_use`、`signaling`）をカスタムクレームに持つ Firebase Auth のカスタムトークンを返す。エラーコードは `docs/edge_bridge_spec.md` のエラー節のとおり

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
