# Edge Bridge Service 入出力仕様（策定中）

package: 未定（WebRTC シグナリングのため Connect-RPC 以外の可能性がある）
実現するコンテキスト: observation_context.md（ドメイン定義 observation_domain.md）
役割: WebRTC のシグナリングの仲介

本仕様は骨子のみである。決まっているのは位置づけと担当範囲で、インタフェースは策定中である。

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

シグナリングの仲介だけを担い、確立後の通信には関与しない。

## 認証・認可（確定分）

- 認証・認可は Firestore のアクセス制御として設計する。内部 JWT の枠組み（internal_jwt.md）の外にあるが、外部資格情報は IdP 発行トークンに一本化する方針は共有する
- 本サービスが `event_access` を直接検証し（IdP の JWKS を参照する。Service Gateway 以外で唯一の参照先）、Firebase Auth のカスタムトークン（`event_id` 等をクレームに含む短命のもの）を発行する
- セキュリティルールがカスタムトークンのクレームと文書パスを突合し、シグナリング文書の読み書きを制限する
- 直接検証で確認する項目: `iss`（IdP）、`aud`（バックエンド API 全体の論理 audience。例 backend-api。Service Gateway が検証する値と同じで、audience はバックエンド API 全体の論理識別子として扱い、本サービス専用の audience は追加しない）、`exp`／`nbf`（clock skew 許容 ±30 秒）、`token_use = event_access`、`event_id`（対象イベントとの一致）、scope（`events.report` または `events.read` を受理する）、`client_id`（発行先 client。受理する client の登録と完全一致すること。受理するのは管理 UI の client とエッジ端末の観測ページの client に限り、未登録の client は scope が足りていても拒否する）。JWKS はキャッシュし、未知の `kid` は即時再取得する（レート制限付き。Service Gateway の規約と同じ）
- `event_access` が鍵束縛（`cnf`（`jkt`））を持つ場合は、DPoP proof も検証する。検証項目は Service Gateway の規約と同じ（proof の署名、トークンの `cnf` と proof の公開鍵の一致、アクセストークンのハッシュの一致、HTTP メソッドと対象 URI の一致、発行時刻の許容窓、proof 識別子の再生防止）。エッジ端末の観測ページは public client であり、発行されるトークンは鍵束縛を持つ
- 失効確認（introspection）は行わず、TTL による自然失効に委ねる。シグナリングは管理系の書き込みではなく、発行するカスタムトークンも短命であるため
- Realtime の Firestore 変更検知も同一方式のアクセス制御を使う（発行は Realtime。Realtime）

## 未策定の事項

- ピアの識別方法と、どのエッジ端末とどのクライアントを結び付けるかの決め方（シグナリング文書の構造を含む）
- カスタムトークン発行の入出力（エンドポイントの形式と proto の要否）
- セキュリティルールの詳細
- STUN／TURN サーバの扱い（自前で持つか、マネージドを使うか）
- 接続の同時数と有効期間の上限

## 関連するドメインイベント

| イベント | 契機 |
|---|---|
| AccessRequested | 観測点起動時に、オーナー／スタッフが管理 UI から WebRTC でグラフ紐づけを行うための接続を要求する |
