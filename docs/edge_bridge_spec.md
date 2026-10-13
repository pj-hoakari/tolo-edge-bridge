# Edge Bridge Service 入出力仕様（策定中）

package: `tolo.edgebridge.v1`
実現するコンテキスト: observation_context.md（ドメイン定義 observation_domain.md）
役割: WebRTC のシグナリングの仲介

本仕様は策定中である。決まっているのは位置づけ、担当範囲、RPC、アクセス制御で、残りは「未策定の事項」に挙げる。

## 位置づけ

WebRTC のシグナリング（SDP・ICE 候補の交換）を仲介する独立したデプロイ単位である。
観測コンテキストに属するが、Observation とは別のマイクロサービスとして分離する。

Service Gateway を経由しない。他のマイクロサービスと異なり、完全に独立した位置に配置する。
したがって spec-base/README.md の「各サービスへの呼び出しは Service Gateway を必ず経由する」規約が明示する例外の1つであり、
service_gateway.md の「マイクロサービスへ Service Gateway を迂回して到達できないことをインフラ層で保証する」の対象外でもある。

## 担う範囲

- WebRTC のシグナリング（接続の確立に必要な情報の交換の仲介）。SDP・ICE 候補の交換は Firestore のリアルタイムリスナーで行い、本サービスはシグナリング用文書の管理とアクセス制御を担う
- シグナリングに用いる Firestore のアクセス制御（カスタムトークンの発行）

## 担わない範囲

- 映像の中継。映像はページ間で直接やりとりする（P2P）。本サービスを通らない
- 映像越しの設定変更。観測点のグラフ紐づけを映像で確認しながら行う操作は、ページ同士で完結する。マイクロサービスを介さない
- エッジ端末のライフサイクル管理。登録・登録解除・Heartbeat は Observation が持つ
- エッジ端末の稼働状態の把握。管理 UI は稼働中（待ち受け中）のエッジ端末を Observation の `ListEdgeDevices` から得る。本サービスは稼働中の端末の一覧を持たず、返さない

シグナリングの仲介だけを担い、確立後の通信には関与しない。

## 入出力一覧

入出力は Connect-RPC とする。決定の根拠は ADR-0085 にある。

| 入出力 | 説明（ユビキタス言語） | 相手 | 認可 | 関連ドメインイベント |
|---|---|---|---|---|
| RequestConnection | 指定したエッジ端末への接続を要求する。セッション文書を作り、`session_id` を返す。tenant と event は `event_access` から取る | 管理 UI | `event_access` + events.read、console 役 | AccessRequested |
| IssueFirestoreToken | シグナリング文書を読み書きするための Firebase Auth カスタムトークンを発行する。tenant と event は `event_access` から取る | 管理 UI、エッジ端末の観測ページ | `event_access` + events.read または events.report、edge 役または console 役 | （発行のみ） |

## ピアの識別（確定分）

- エッジ端末側のピアは `edge_device_id`（Observation が発行する公開 ID）で識別する。観測ページは発行 URL に含まれる `edge_device_id` を使い、端末側のストレージに別の識別子を持たない
  `edge_device_id` を持たない観測ページは WebRTC で待ち受けない
- エッジ端末の稼働中と WebRTC の待ち受け中は同じ状態である。観測ページは待ち受けている間だけ Observation へ Heartbeat を送る（Observation）
- 管理 UI は Observation の `ListEdgeDevices` で稼働中のエッジ端末を得て、その `edge_device_id` を指定して接続を要求する
- 接続中は、管理 UI とエッジ端末の P2P 通信が確立している状態を指す。管理 UI 側の状態であり、本サービスも Observation も持たない
- 本サービスは接続要求の受け付け時に、エッジ端末の稼働を確かめない。待ち受けをやめた直後の端末は、途絶の判定までの最大30秒は稼働中として表示され、その間の接続要求は確立しない

## 認証・認可（確定分）

- 認証・認可は Firestore のアクセス制御として設計する。内部 JWT の枠組み（internal_jwt.md）の外にあるが、外部資格情報は IdP 発行トークンに一本化する方針は共有する
- 本サービスが `event_access` を直接検証し（IdP の JWKS を参照する。Service Gateway 以外で唯一の参照先）、Firebase Auth のカスタムトークン（`event_id` 等をクレームに含む短命のもの）を発行する
- セキュリティルールがカスタムトークンのクレームと文書パスを突合し、シグナリング文書の読み書きを制限する
- 直接検証は、本文をデコードする前の HTTP middleware で、全 RPC に共通して行う。確認する項目: `iss`（IdP）、`aud`（バックエンド API 全体の論理 audience。例 backend-api。Service Gateway が検証する値と同じで、audience はバックエンド API 全体の論理識別子として扱い、本サービス専用の audience は追加しない）、`exp`／`nbf`（clock skew 許容 ±30 秒）、`token_use = event_access`、`tenant_id` と `event_id`（`event_access` が持つこと。リクエストに対象イベントを含めないため、突き合わせる値はない）、scope（`events.report` または `events.read` を受理する）、`client_id`（発行先 client。受理する client の登録と完全一致すること。受理するのは管理 UI の client とエッジ端末の観測ページの client に限り、未登録の client は scope が足りていても拒否する）。JWKS はキャッシュし、未知の `kid` は即時再取得する（レート制限付き。Service Gateway の規約と同じ）
- `event_access` が鍵束縛（`cnf`（`jkt`））を持つ場合は、DPoP proof も検証する。検証項目は Service Gateway の規約と同じ（proof の署名、トークンの `cnf` と proof の公開鍵の一致、アクセストークンのハッシュの一致、HTTP メソッドと対象 URI の一致、発行時刻の許容窓、proof 識別子の再生防止）。対象 URI は、設定した公開 URL と RPC のパスから組み立て、任意の Host ヘッダと転送ヘッダは信頼しない。エッジ端末の観測ページは public client であり、発行されるトークンは鍵束縛を持つ
- 役割（edge 役、console 役）の判定も middleware で行う。`RequestConnection` は console 役だけに許可する
- 失効確認（introspection）は行わず、TTL による自然失効に委ねる。シグナリングは管理系の書き込みではなく、発行するカスタムトークンも短命であるため
- Realtime の Firestore 変更検知も同一方式のアクセス制御を使う（発行は Realtime。Realtime）

## カスタムトークン（確定分）

- uid は `event_access` の `sub` を写す
- カスタムクレームは次のとおりとする

| クレーム | 値 |
|---|---|
| `tenant_id` | `event_access` の `tenant_id` |
| `event_id` | `event_access` の `event_id` |
| `peer_role` | `edge` または `console`。検証した `client_id` から決める |
| `expires_at` | `event_access` の `exp`（UNIX 時刻、秒）。Firestore へアクセスできる期限 |

- Firebase のセッションはリフレッシュトークンで更新され続けるため、カスタムトークンの有効期間（発行から 1 時間）は Firestore へアクセスできる期間の上限にならない。上限は `expires_at` とし、セキュリティルールで打ち切る
- レスポンスの `expires_at` はクレームの `expires_at` と同じ時刻を指す。クライアントは、この時刻の前に新しい `event_access` でカスタムトークンを取り直し、サインインし直す

## エラー

Service Gateway と同じ Connect エラーコードで返す。外部レスポンスの本文に tenant、event、client の詳細を含めない。

| 事象 | Connect エラーコード |
|---|---|
| トークンがない、または無効、期限切れ。`token_use` の不一致。`tenant_id` か `event_id` がない | unauthenticated |
| DPoP proof の欠落、無効、再生、または鍵の不一致（`cnf` を持つトークンのみ） | unauthenticated |
| `client_id` が受理する client にない。scope が足りない。役割が RPC に許可されない | permission_denied |
| JWKS を取得できない（未知の `kid` の再取得を控えている間を含む） | unavailable |
| `RequestConnection` の `edge_device_id` が空 | invalid_argument |

## シグナリング文書とセキュリティルール（確定分）

決定の根拠と、受容したリスクは ADR-0084 にある。

- シグナリング文書は次のパスに置く。`tenantId`、`eventId`、`edgeDeviceId` は公開 ID である
  `tenants/{tenantId}/events/{eventId}/edgeDevices/{edgeDeviceId}/sessions/{sessionId}/signals/{signalId}`
- 待ち受け側（edge 役）はエッジ端末の観測ページの client、接続要求側（console 役）は管理 UI の client とする。本サービスは、検証した `client_id` を受理する client の設定と突き合わせて役割を決める
- カスタムトークンのクレームは前節のとおりとする。`edge_device_id` はクレームに含めない
- セッション文書は、本サービスが接続要求を受け付けたときに作る（Admin SDK）。接続要求は console 役だけが行え、tenant と event はリクエストではなく `event_access` から取る
- signals は、`from`（送り手の役割）、`payload`（SDP または ICE 候補）、`createdAt` を持つ文書として追記する
- セキュリティルールは、パスの `tenantId` と `eventId` がクレームと一致すること、`request.time` がクレームの `expires_at` より前であることをすべての許可の条件とし、そのうえで次を許す。これ以外のパスと操作は拒否し、コレクショングループクエリも許可しない

| 対象 | 読み取り | 作成 | 更新 | 削除 |
|---|---|---|---|---|
| sessions | edge 役、console 役 | なし（本サービスの Admin SDK のみ） | なし | console 役 |
| signals | edge 役、console 役 | edge 役、console 役。`from` が自分の役割と一致し、フィールドが `from`、`payload`、`createdAt` に限られ、`createdAt` が書き込み時刻であること | なし | console 役 |

- edge 役は同じイベントのどのエッジ端末のパスにも読み書きできる。`edge_device_id` は資格情報ではないため、同じイベント内での端末のなりすましは防がない。影響はそのイベントの範囲に閉じる

## 未策定の事項

- 閉じられずに残ったセッションの片付け（TTL やサーバ側の削除）
- STUN／TURN サーバの扱い（自前で持つか、マネージドを使うか）
- 接続の同時数と有効期間の上限

## 参考 proto 定義

```proto
syntax = "proto3";
package tolo.edgebridge.v1;
import "google/protobuf/timestamp.proto";

service SignalingService {
  // 管理 UI → 本サービス。console 役のみ
  rpc RequestConnection(RequestConnectionRequest) returns (RequestConnectionResponse);
  // 管理 UI、観測ページ → 本サービス。edge 役、console 役
  rpc IssueFirestoreToken(IssueFirestoreTokenRequest) returns (IssueFirestoreTokenResponse);
}

message RequestConnectionRequest {
  string edge_device_id = 1;
}
message RequestConnectionResponse {
  string session_id = 1;
}

// tenant と event は event_access から取る
message IssueFirestoreTokenRequest {}
message IssueFirestoreTokenResponse {
  string firebase_custom_token = 1;  // Firebase Auth の signInWithCustomToken に渡す
  google.protobuf.Timestamp expires_at = 2;  // クレームの expires_at と同じ時刻
}
```

## 関連するドメインイベント

| イベント | 契機 |
|---|---|
| AccessRequested | 観測点起動時に、オーナー／スタッフが管理 UI から WebRTC でグラフ紐づけを行うための接続を要求する。本サービスが接続要求を受け付け、セッション文書を作った時点にあたる |
