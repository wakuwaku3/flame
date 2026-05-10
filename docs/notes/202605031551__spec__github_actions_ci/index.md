# GitHub Actions による CI 検査ワークフローの整備 (spec)

## 目的

flame の Stop hook で実行している静的検査 ([FLM_GEN_0003](../../adr/general/FLM_GEN_0003__feedback_loop.md) の 1 層目) を、3 層モデルの 2 層目である CI でも実行できるようにする。AI を介さない直接編集 / hook がスキップされたケース / AI のハルシネーションに備え、main へのマージ前の最後の砦として機能させる。

併せて GitHub Actions ワークフロー自体を flame の content type として扱い、ADR・命名規約・静的検査・関連 skill を整備する。

## 対象範囲

- `pull_request_target` を起点とする CI ワークフローの新設
- `scripts/check.sh` 相当の静的検査の CI 実行 (種別ごとの並列化を含む)
- ワークフローファイルの配置・命名規約とその ADR 化
- ワークフローファイル自体に対する静的検査機構
- ワークフロー作成 / トリガーテストのための skill

## 非対象

- Stop hook で実行している AI レビュー段 (general-practices-reviewer / adr-reviewer / rule-adr-sync-reviewer) の CI 化。AI レビューは Stop hook 側に固定し、CI に移植しない (hook は CI 整備後もそのまま運用される)。理由: AI レビューは AI 開発ループ内で「次の応答を返す前に AI 自身が直す」ことに価値があり、PR レビュー段階に移すと AI が修正できない位置で違反が顕在化する
- 監視層 ([FLM_GEN_0003](../../adr/general/FLM_GEN_0003__feedback_loop.md) の 3 層目) の整備
- 重い検査 (E2E、複数環境ビルド等) の追加。現状の flame には存在しないため

## 全体像

```text
PR 作成/更新
   │
   ▼
.github/workflows/trg__pull_request_target__opened_synchronize_reopened.yaml   (trigger)
   │  ・pull_request_target の activity types を受ける薄い層
   │  ・workflow_call で wf__check.yaml を呼ぶ
   ▼
.github/workflows/wf__check.yaml                                                (aggregator)
   │  ・wf__check__diff.yaml を呼んで対象ファイル一覧を取得
   │  ・detect.sh 相当で種別ごとにファイル群を分け
   │  ・matrix strategy で各 check-*.sh を並列実行
   ▼
   ├─ wf__check.yaml の matrix job ─┐
   │   check-document               │
   │   check-adr                    │
   │   check-shell                  │ (種別ごとに独立した job、fail-fast: false)
   │   check-json                   │
   │   check-yaml                   │
   │   check-devbox                 │
   │   check-flow-document          │
   └────────────────────────────────┘

   ※ wf__check__diff.yaml は wf__check.yaml から呼ばれる差分計算専用 reusable workflow。
   　 単独でも workflow_dispatch から起動でき、テスト時に独立検証できる。
```

## ファイル配置と命名規約

すべて `.github/workflows/` 配下。拡張子は `.yaml` を採用する (リポジトリ既存の `.markdownlint-cli2.yaml` 等と揃え、flame 内の YAML 拡張子を `.yaml` に統一する)。

[FLM_APP_0004](../../adr/application/FLM_APP_0004__yaml.md) は「外部ツールが `.yml` を要求する場合は `.yml` を許容する」とし、その例として GitHub Actions を挙げているが、GitHub Actions は実際には `.yml` / `.yaml` の両方を読み込む (要求ではない)。この事実誤認を是正するため、本 spec から派生する ADR で FLM_APP_0004 の当該記述を改修する (詳細は「未確定事項」参照)。

### トリガーワークフロー (`trg__*.yaml`)

- 形式: `trg__{event}__{discriminator}.yaml`
  - `event`: GitHub Actions の event 名 (例: `pull_request_target`, `push`, `schedule`, `release`)
  - `discriminator`: 当該 event を一意に区別する snake_case 文字列。意味は event 種別ごとに異なる:
    - activity types を持つ event (`pull_request_target` / `issues` / `release` 等): types を `_` 連結 (例: `opened_synchronize_reopened`)
    - branch / tag / path filter を持つ event (`push` / `pull_request` 等): filter 対象を表す snake_case (例: `main`, `all`, `release_branches`)
    - 周期実行 (`schedule`): スケジュール識別子 (例: `daily`, `weekly`)
    - その他 (`workflow_run` 等): 起動元・用途を表す snake_case
  - 例:
    - `trg__pull_request_target__opened_synchronize_reopened.yaml`
    - `trg__push__main.yaml` (main への push のみ)
    - `trg__push__all.yaml` (全ブランチへの push)
    - `trg__schedule__daily.yaml`
- 役割: イベント受信のみ。検査ロジックは持たず、`workflow_call` で 1 本の `wf__*` を呼ぶだけの薄い層に保つ (= trigger 層は常に単一 reusable workflow への dispatch に限定する。複数 wf を順序づけて呼ぶ必要がある場合は、その合成自体を 1 本の `wf__*` 内で表現する)
- 静的検査が許容する内容: `name` / `on` / `jobs.*.uses` / `jobs.*.with` / `jobs.*.secrets` / `jobs.*.permissions` のみ。`steps`・`run`・自前の `runs-on`・`jobs.*.needs` は禁止する (= 単一 reusable workflow を呼ぶだけの形に強制)
- 他リポジトリ / 他ワークフローからの再利用は想定しない (各リポジトリでイベント仕様に合わせて配置する)

### 実体ワークフロー (`wf__*.yaml`)

- 形式: `wf__{verb}.yaml` または `wf__{verb}__{target}.yaml`
  - `verb`: 動作を表す動詞 (例: `check`, `build`, `deploy`, `release`)
  - `target`: 動作対象を表す snake_case (省略可能)。動詞単独で意味が成立する場合は省略してよい
  - 例 (本 spec のスコープ内):
    - `wf__check.yaml` (差分対象に対する全種別検査の aggregator)
    - `wf__check__diff.yaml` (差分対象ファイル一覧の計算)
  - 命名規約説明用の例 (本 spec のスコープ外、整備対象ではない):
    - `wf__check__all.yaml` (全件検査の手動 dispatch を将来欲しくなった場合の命名例)
- 必須要素:
  - `on.workflow_call` を備える (トリガー層・他 wf から呼ばれる経路)
  - `on.workflow_dispatch` を備える (手動実行・テスト用)
  - 入力パラメータは `workflow_call.inputs` と `workflow_dispatch.inputs` で同名同型に揃える
- トリガーとの分離方針: 実体ワークフローはイベントに依存しない (= `on.pull_request_target` 等を直接書かない)。トリガー層がイベント差分を吸収する
- wf 同士の合成: 1 つの実体ワークフローが他の実体ワークフローを `workflow_call` で呼んでよい (例: `wf__check.yaml` が `wf__check__diff.yaml` を呼ぶ)。階層の深さに上限は置かない。合成の意図をワークフロー先頭コメントに記すことを推奨するが、静的検査での強制はしない

## ワークフロー構成

### `wf__check__diff.yaml` (差分計算)

責務: base / head SHA を受け、変更ファイル一覧を返すだけの単一責務 reusable workflow。

- 入力 `base_sha` / `head_sha` を `workflow_call.inputs` / `workflow_dispatch.inputs` の両方で受け取る。両入口とも `required: true` (固定 SHA を必須化、デフォルト値は持たない。ref 名・暗黙の解決は受けない)
- `actions/checkout` は `fetch-depth: 0` で base / head 双方を取得する
- `git diff --name-only --diff-filter=ACMR "$BASE_SHA...$HEAD_SHA"` で対象ファイル一覧を取得 (3 ドット = マージベースからの差分)
- 削除ファイルは検査対象から除外 (`--diff-filter=ACMR` で D を除外)
- 出力: `workflow_call.outputs.files` (フォーマット未確定。「未確定事項」参照)

### `wf__check.yaml` (aggregator)

責務: 差分対象に対する全種別検査の総合エントリ。トリガー層から呼ばれる主入口。

- 入力 `base_sha` / `head_sha` を `workflow_call.inputs` / `workflow_dispatch.inputs` の両方で受け取る (両入口とも `required: true`、`wf__check__diff.yaml` と同じ契約)
- 内部で `wf__check__diff.yaml` を `workflow_call` で呼んで変更ファイル一覧を取得する
- 取得したファイル一覧を `scripts/detect.sh` 相当のロジックで種別ごとに分割する (= job 1 段)
- 出力 (種別 → ファイル一覧の JSON マップ) を `strategy.matrix` に渡し、各種別の `check-*.sh` を独立した job として並列実行する
- 1 種別の失敗で他種別の job を打ち切らない (`fail-fast: false`)
- 変更ファイル 0 件のときも matrix を空起動せず、no-op の success job を 1 個返す (branch protection の required status check が「未実行」で待ち続けないようにするため)

種別と checker の対応は `scripts/detect.sh` を信頼の単一情報源 (SoT) として再利用する。CI 側で種別判定ロジックを再実装しない。

### SHA 入力の取得元

`pull_request_target` 経路から `wf__check.yaml` を呼ぶ場合 (trigger 層が以下を `with:` で渡す):

- base: `${{ github.event.pull_request.base.sha }}`
- head: `${{ github.event.pull_request.head.sha }}` (イベント時点で固定された SHA。PR 更新中の race condition を避けるため `head.ref` の最新解決ではなくこの固定値を使う)

`workflow_dispatch` 経路では呼び出し側 (人間 / `gh workflow run`) が base / head の固定 SHA を必ず手動指定する。デフォルト値や暗黙のフォールバック (例: 「現在の HEAD と main の merge-base」など) は提供しない (動作の決定論性を保つため)。

### shell injection への防御

外部入力の SHA は `pull_request_target` 経路で fork からの値を含み得るため、すべて `env:` 経由で渡し `run` 内では必ずダブルクオートして展開する。`${{ }}` 展開を `run` の本文に直接埋め込まない。

```yaml
- name: Compute diff
  env:
    BASE_SHA: ${{ inputs.base_sha }}
    HEAD_SHA: ${{ inputs.head_sha }}
  run: git diff --name-only --diff-filter=ACMR "$BASE_SHA...$HEAD_SHA"
```

## ADR / 検査機構 / skill の整備

GitHub Actions ワークフローを flame の content type として扱い、[FLM_GEN_0005](../../adr/general/FLM_GEN_0005__content_type.md) の 5 項目で整備する。

### ADR (新規)

- ファイル: `docs/adr/engineering/FLM_ENG_{NNNN}__github_actions.md` (採番は本 spec 確定時に決定)
- 決定事項として書く内容:
  - ワークフロー配置: `.github/workflows/` 配下
  - 拡張子: `.yaml`
  - 命名規約: `trg__{event}__{discriminator}.yaml` / `wf__{verb}[__{target}].yaml`
  - トリガーと実体の分離方針 (実体は `workflow_call` + `workflow_dispatch` を必ず備える、trigger は reusable workflow を呼ぶだけの薄い層)
  - 差分スコープ: 3 ドット diff (`base...head`)、固定 SHA 入力、shell injection 防御 (env 経由)
  - 並列化方針: 種別ごとの matrix
  - YAML 全般のルール ([FLM_APP_0004](../../adr/application/FLM_APP_0004__yaml.md)) を継承
- 抽象 policy のみ書き、実装詳細 (具体的な action のバージョン等) は背景・影響に書く ([FLM_GEN_0001](../../adr/general/FLM_GEN_0001__adr.md))

### FLM_APP_0004 (YAML) の改修

本 spec の ADR 化と同時に [FLM_APP_0004](../../adr/application/FLM_APP_0004__yaml.md) を改修する。

- 現状の記述「外部ツールごとに要求する拡張子が異なる (例: GitHub Actions は `.yml`、markdownlint-cli2 は `.yaml`)」のうち、GitHub Actions は実際には両対応であり「要求」ではない事実を反映する
- flame では `.yaml` を原則とし、`.yml` は「ツール側が `.yaml` を読まないと判明している場合」のみに限定する旨を明確化する
- 既存ファイルへの影響: `.markdownlint-cli2.yaml` は変更不要。GitHub Actions 配下を `.yaml` で統一して整合させる

### lint / 静的検査

優先順位は [FLM_GEN_0004](../../adr/general/FLM_GEN_0004__static_check.md) に従う。

- 既存ツール: [actionlint](https://github.com/rhysd/actionlint) を採用 (構文・shell ブロック・式エラー等を検査)
- カスタム静的ルール (`scripts/check-github-actions.sh` 新設):
  - ファイル名が `trg__*.yaml` または `wf__*.yaml` のいずれかに合致
  - `wf__*.yaml` は `on.workflow_call` と `on.workflow_dispatch` の両方を持つ
  - `trg__*.yaml` の `event` 部分が GitHub Actions の有効な event 名
  - `trg__*.yaml` は実体ロジックを持たず `uses: ./.github/workflows/wf__*.yaml` で呼ぶだけ (許可キー集合: `name` / `on` / `jobs.<id>.uses` / `with` / `secrets` / `permissions`。`steps` / `run` / `runs-on` / `needs` は禁止。`jobs` の数は 1 のみ)
  - `trg__*` の `discriminator` がファイル内 `on.{event}` の types / branches 等の filter 設定と矛盾しない。対応関係:
    - activity types を持つ event: discriminator が `_` 連結された types 一覧と `on.{event}.types` が完全一致
    - branch / tag filter を持つ event: discriminator のセマンティクスと `on.{event}.branches` (または `tags`) が一致 (例: `trg__push__main.yaml` は `on.push.branches: [main]` のみ、`trg__push__all.yaml` は `branches` 未指定または `'**'`)
    - schedule: discriminator は識別子で filter との対応関係はなし (cron 式自体は中身で表現)
- `scripts/detect.sh` に `is_github_actions_workflow` を追加して dispatch を組み込む

### test (トリガーのテスト)

- 自動 test の範囲: [act](https://github.com/nektos/act) で `trg__*.yaml` をローカル / CI 上で発火させ、想定する `wf__*.yaml` を期待 inputs で呼び出すかを検証する。これに限定する
- 自動 test の対象外: `wf__*.yaml` 単独の動作検証。実体ロジックの確認は GitHub 上での `workflow_dispatch` (例: `gh workflow run wf__check.yaml -f base_sha=... -f head_sha=...`) による手動運用、および受け入れ基準 6 (代表 PR でのローカル / CI 一致確認) でカバーする。act でも `wf__*` を直接叩けるが、依存 action の挙動差で偽陰性を生むリスクがあるため自動 test には組み込まない
- test 本体は shell script (`scripts/test-github-actions-trigger.sh` 等) として配置し、devbox に act を追加する ([FLM_ENG_0002](../../adr/engineering/FLM_ENG_0002__devbox.md))

### skill

- 作成 skill (`.claude/skills/github-actions/SKILL.md`): ワークフロー新規作成時に呼ぶ procedural skill。命名規約・必須要素・lint / act 検証手順を案内する
- ADR ルール検査 skill: 静的検査でカバーできるため省略 ([FLM_GEN_0004](../../adr/general/FLM_GEN_0004__static_check.md) の方針に従う)

### rule (`.claude/rules/`)

- `.claude/rules/github-actions.md` を新設し `paths: [".github/workflows/**"]` で auto-inject させる
- 内容は ADR へのリンク + 1〜2 行の要約 ([FLM_ENG_0001](../../adr/engineering/FLM_ENG_0001__claude_code.md) のルール構成方針に従う)

### 5 項目の整備状況サマリ

| 項目 | 整備方針 |
| --- | --- |
| 作成 skill | `.claude/skills/github-actions/SKILL.md` を新設 |
| lint | actionlint + カスタム静的検査 (`check-github-actions.sh`) を hook と CI で実行 |
| build | 省略 (静的設定のため) |
| test | act によるトリガー発火テスト |
| ADR ルール検査 skill | 省略 (lint で完結) |

## 受け入れ基準

1. `.github/workflows/` 配下に `trg__pull_request_target__*.yaml` (具体名は ADR 確定後に決定) が存在し、PR の opened / synchronize / reopened を受けて `wf__check.yaml` を `workflow_call` で呼ぶ
2. `.github/workflows/wf__check.yaml` および `wf__check__diff.yaml` が存在し、それぞれ `workflow_call` と `workflow_dispatch` の両方で起動できる
3. `wf__check.yaml` は内部で `wf__check__diff.yaml` を呼び、変更ファイルに対してのみ各 checker を実行する
4. PR を opened / synchronize / reopened で更新すると CI が起動し、変更ファイルに対してのみ checker が走る
5. 種別ごとに job が分割され、並列実行される (1 種別の失敗で他が打ち切られない)
6. 各 content type を 1 ファイルずつ含む代表 PR (= テストフィクスチャ) に対し、ローカル `bash scripts/check.sh <変更ファイル>` の終了コードと、CI の `wf__check.yaml` 実行の matrix job 群を集約した最終終了コードが一致する (出力フォーマットは GitHub Actions annotations 等で差分が出るため文字列比較はしない)
7. ワークフロー配置・命名・必須要素を ADR 化し、`.claude/rules/github-actions.md` から auto-inject される
8. `scripts/check-github-actions.sh` がローカルおよび CI で `trg__*` / `wf__*` の規約違反を検出する
9. act によるトリガー test が devbox 経由で再現できる

## 制約条件

受け入れ基準として検証するものではないが、実装が満たすべき非機能要件として以下を置く。

- 既存の Stop hook 動作 (`scripts/stop-hook-review.sh` および `scripts/check.sh` の入出力契約) を破壊しない。CI 整備による退行を許容しない
- AI レビュー段は Stop hook 側に固定され、CI 整備によって hook 側の責務が削られない (hook で AI レビューを継続実行する)

## 依存タスク (本 spec のスコープ外だが ADR 化と連動して進める)

- [FLM_APP_0004](../../adr/application/FLM_APP_0004__yaml.md) の改修: 「GitHub Actions が `.yml` を要求する」という事実誤認の是正、および本 spec で `.yaml` を採用したことに伴う ADR 側の例示 / 規定文の更新。本 spec の ADR 化と同一サイクルで実施する想定だが、本 spec の受け入れ基準には含めない (派生 ADR タスクとして独立管理)

## 未確定事項 / リスク

- **`pull_request_target` のセキュリティ**: `pull_request_target` は base branch のワークフローを使うため fork からの未信頼コードを直接実行しないが、checkout で head SHA を取り込んだ後に shell を走らせる構造のため、checker 自体に注入経路がないかは別途確認が必要。`pull_request` (head のワークフローを使う) との比較は ADR の「評価」セクションで論点として扱う想定
- **削除ファイルの扱い**: 削除ファイルは検査対象外 (`--diff-filter=ACMR`) としたが、削除によって参照切れが発生する Markdown リンク等は検出できない。これは現状の Stop hook も同等の振る舞いのため許容するが、後続で扱うか要検討
- **discriminator 命名の自由度**: `discriminator` を「event を一意に区別する snake_case 文字列」とだけ規定したため、書き手によって命名がぶれる余地がある (例: `main` への push を `trg__push__main.yaml` とするか `trg__push__main_only.yaml` とするか)。ADR で代表ケースの命名サンプルを列挙して揺れを抑える方針
- **複合 discriminator**: 1 つの event に対して activity types と branch filter の両方で区別したい場合 (例: 「main 向けの opened/synchronize PR」だけを受けたいケース) の命名が未定義。ADR で「複合は意味のある語にまとめる (例: `opened_to_main`)」「複合させず 2 ファイルに分ける」のいずれかを決定する
- **types 列の長さ**: `trg__pull_request_target__opened_synchronize_reopened.yaml` のように types を全列挙するとファイル名が長大になる。代替として「主要 types のみ書く」「複数ワークフローに分ける」「types 不問の汎用名 (例 `trg__pull_request_target__all.yaml`) にする」などを ADR で評価する
- **`scripts/detect.sh` の SoT 化**: CI から detect.sh を呼ぶ前提だが、種別 → ファイル一覧の出力フォーマットを CI が消費しやすい JSON 等に拡張するか、現状のタブ区切りのままパースするかは実装時に決める
- **`wf__check__diff.yaml` の outputs 形式**: 改行区切り文字列 / JSON 配列 / `${{ toJson(...) }}` 経由のいずれを採用するかは実装時に決定。matrix への引き渡しやすさを優先する
