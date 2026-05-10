# パスベースラベル付与ワークフロー

## 背景

- flame は GitHub Label を自動付与方式で運用する ([FLM_ENG_0004](../engineering/FLM_ENG_0004__github_label.md))。 自動付与 label は `<category>/<value>` 形式で、 現時点の category として `module` を採用している
- flame の CI 検査は PR の `pull_request` の opened / synchronize / reopened を契機に走り、 base SHA / head SHA から 3 ドット diff によって変更ファイル一覧を抽出する ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md))
- GitHub Actions の `pull_request` event のデフォルト `GITHUB_TOKEN` は同一リポジトリ PR では `pull-requests: write` を要求できるが、 fork リポジトリ由来 PR では read-only に絞られる ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md))
- flame では top-level ディレクトリで `go.mod` を持つものを 1 つの module として扱う ([FLM_APP_0007](../application/FLM_APP_0007__go.md))
- GitHub CLI (`gh`) は label の作成 (`gh label create`) と PR への label 付与 (`gh pr edit --add-label`) を提供する。 label が repo に未存在のときは作成が必要となる ([FLM_ENG_0004](../engineering/FLM_ENG_0004__github_label.md))
- flame では補助ドキュメント (rule / skill / CLAUDE.md / README) を ADR の従属物として位置付ける ([FLM_GEN_0001](../general/FLM_GEN_0001__adr.md))
- flame ではコンテンツ種別ごとに「作成 skill / lint / build / test / ADR ルール検査 skill」の 5 項目を整備する規約がある ([FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md))

## 決定

flame では PR の変更ファイルパスから自動付与 label ([FLM_ENG_0004](../engineering/FLM_ENG_0004__github_label.md)) を derive する経路を以下のルールで運用する。 本 ADR は path-based 自動付与の方式に限定し、 label 一般の policy ([FLM_ENG_0004](../engineering/FLM_ENG_0004__github_label.md)) を再記述しない。

### 起動契機

- PR の opened / synchronize / reopened を契機に走る
- 既存の CI 検査 ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md)) と並列に同 PR トリガから起動する。 CI 検査と独立した別トリガーは作らず、 同 PR トリガから検査 / label の両方を派生させる
- 起動契機の event を増やしたい場合は本 ADR を更新する

### path → label マッピング規則

- 本経路で扱う category は `module` ([FLM_ENG_0004](../engineering/FLM_ENG_0004__github_label.md)) のみ
- マッチ規則: 変更ファイルパスが `<module>/...` の形 (top-level の `go.mod` を持つディレクトリ直下に位置する) の場合、 当該 module の `module/<module>` label を付与する
- マッチしない top-level ディレクトリ (例: `docs/`、 `scripts/`、 `.github/`) には label を付与しない
- 1 PR で複数 module が触れられた場合は対応する複数 label を並べて付与する

### 既存 label との差分処理

- 既存 label は消さず、 不足している label のみ追加する
- 後続 push でファイル変更が module の外に移った場合も、 過去の付与は手動操作のために残置する

### Fork PR での挙動

- fork PR ではデフォルト `GITHUB_TOKEN` が read-only に絞られるため label 付与は走らせず、 ジョブ全体を skip する
- skip しても CI ジョブの集約 status は success を返す。 label 付与失敗を branch protection に伝播させない

### 5 項目の整備状況

[FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md) で定める 5 項目について以下を整備する。 本経路は GitHub Actions ワークフロー ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md)) と shell スクリプト ([FLM_APP_0002](../application/FLM_APP_0002__shell_script.md)) で実装するため、 種別固有の整備は持たない。

| 項目 | 整備方針 |
| --- | --- |
| 作成 skill | 省略 (実装は GitHub Actions skill と shell スクリプト skill に委譲) |
| lint | 省略 (実装側の YAML / shell の各 lint に委譲) |
| build | 省略 (静的設定のため出力生成の概念がない) |
| test | 省略 (実 label 付与は PR 上でのみ起こるため、 ローカル test は持たない) |
| ADR ルール検査 skill | 省略 (実装側の lint に委譲) |

## 影響

- PR ごとに「どの module を触ったか」 が UI 上で一目で分かる
- 自動 label は変更ファイルパスから機械的に決まるため、 PR 著者が label 付与を意識する必要がない
- 同一 PR が複数 module を触った場合は複数の `module/<name>` label が並ぶ
- module ディレクトリを新設・改名した場合、 当該変更の PR には新しい module label が付き、 後続 PR には改名後 label が付く。 過去 PR の label は遡って書き換えない (consumer 側 ([FLM_FEA_0004](FLM_FEA_0004__release_policy.md)) で過去 PR を参照する場合、 当該 PR が古い名前の label を持っていれば旧名で検索する必要がある)
- fork PR では token 制約により label が付かないため、 label 消費機構 ([FLM_FEA_0004](FLM_FEA_0004__release_policy.md) 等) は fork PR を取りこぼす可能性がある (現状の flame には fork 経由 PR を扱う想定が無い)
- top-level ディレクトリの新設で「`<dir>/...` を触ったが label が付かない」 PR が出ても、 label 消費機構は当該 PR を取りこぼすだけで CI は fail しない (label 生成と消費機構は互いに decouple している)
- label 付与は PR の opened / synchronize / reopened ごとに走るため、 後追いで変更ファイルが増えた場合も追加 label が反映される
- 起動契機を CI 検査と同 PR トリガに同居させるため、 検査 workflow と label workflow の dispatch 経路は単一トリガーから派生する。 トリガー追加・削除は label 付与の起動契機と検査の起動契機を同時に動かす形になる
- 「決定」 で抽象 policy として残した具体実装 (label workflow の名前、 切り出しスクリプトの配置、 label 自動作成時の色 / description、 起動契機の同居形態) は workflow 定義 / スクリプト本体に分散して保持される。 仕様変更時に各実装側で更新する

## 評価

代替案として以下を検討した。

- **path-based label 付与を別の専用トリガから走らせる (CI 検査と並列に独立 trigger を新設)**: 検査 workflow と label workflow を完全に decouple できる利点があるが、 (1) `pull_request` の opened / synchronize / reopened は CI 検査と同じ起動契機で、 トリガーを 2 本立てると同条件のトリガーが repo 内で重複する、 (2) [FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md) のトリガー層命名規約 (`trg__<event>__<discriminator>.yaml`、 discriminator は activity types を `_` 連結) で同じ types 集合を持つトリガーは file 名が衝突する、 (3) label と検査は同じ変更 file 一覧を入力に使うため、 同じトリガーから派生させた方が diff 計算を共有できる。 既存 PR トリガから検査 / label の両方を派生させる構造を採用した。
- **path-based 以外に commit author / branch 名 / PR タイトルから label を derive する経路を本 ADR に同居させる**: 1 ADR で複数の derive 方式を網羅できる利点があるが、 各方式で起動契機・mapping 規則・consumer が独立して進化するため、 1 ADR に詰め込むと改訂のたびにスコープが広がる。 本 ADR は path-based に限定し、 別方式を導入する場合は別 ADR で扱う方を採用した。
- **`<module>/` 直下のパスマッチではなく、 prefix の一致だけでマッチさせる (例: `<module>foo/bar` も hit させる)**: マッチが緩く実装が短い反面、 `cli` module を導入した repo で `client/` のような他 dir も `cli` と誤一致する。 `<module>/` (末尾スラッシュ含む) でマッチを取り、 ディレクトリ単位で厳密に判定する方を採用した。
- **`<module>/` 直下に該当しないネストパス (例: `<module>/cmd/<name>_tool/...`) を検出から外す**: 厳密に「変更が module 直下のみ」 を取りたい局面はあるが、 release ノート生成は当該 module 全体に効く変更を拾いたい (cmd / internal / scripts いずれも変更時に release が走る)。 `<module>/` で始まるパス全てを当該 module の変更とみなす方を採用した。
- **既存 label を「現在の touch 状況」 と同期して削除する (touch から外れた label を消す)**: PR の現状を正確に label に反映できる利点があるが、 (1) 人間が任意に追加した label まで誤って削除する危険がある、 (2) PR の途中で label を一時的に外す運用 (レビュー段階のフラグ等) が壊れる。 削除はせず追加のみ行う方を採用した。
- **fork PR で label を付けるため `pull_request_target` event を併用する**: fork PR からも label が付くが、 base ブランチ workflow が PR 著者の任意コードを実行する経路が復活し、 [FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md) で `pull_request_target` を撤去した動機 (信頼分割プラミングを削る) と整合しない。 fork PR は label を諦め、 自動 label 機構は同 repo PR のみを対象にした。

過去に採用していた決定として以下の経緯がある。

- 当初は path-based 自動付与の規約を [FLM_ENG_0004](../engineering/FLM_ENG_0004__github_label.md) (GitHub Label の運用) に同居させていた。 path-based は label 自動付与方式の 1 つに過ぎず、 「label をどう運用するか (一般)」 と「path から label を derive する具体方式」 を分けた方が将来別方式 (commit author / branch 名 / PR タイトル等からの derive) を追加するときの吸収余地が広がるため、 本 ADR として独立させた。 [FLM_ENG_0004](../engineering/FLM_ENG_0004__github_label.md) には label 一般の policy のみを残し、 本 ADR は path-based 方式の決定のみを扱う。
