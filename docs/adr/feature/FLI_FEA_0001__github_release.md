# flame の GitHub Release 経路の実装

## 背景

- GitHub Release を経由して semver 採番された配布対象を release する一般 policy は [FLM_FEA_0004](../../../vendor/flame/docs/adr/feature/FLM_FEA_0004__release_policy.md) で定義されている。 本 ADR は当該 policy を base に、 flame self における具体実装 (配布対象の具体名・workflow ファイル名・flame CLI subcommand 名・install スクリプト path・dry_run 入力等) のみを規定する
- flame の配布対象 main package は `<module>/cmd/<app_dir>/` に置かれ、 `<app_dir>` は `_tool` suffix を持つ ([FLM_APP_0007](../../../vendor/flame/docs/adr/application/FLM_APP_0007__go.md) §配置)
- flame の library 配布対象は module ディレクトリ名が `lib` または `_lib` suffix を持つ ([FLM_APP_0007](../../../vendor/flame/docs/adr/application/FLM_APP_0007__go.md) §配置)
- flame は補助処理を集約する flame CLI を持つ ([FLI_FEA_0002](FLI_FEA_0002__flame_cli.md))
- flame の自動化処理は shell スクリプトで実装している ([FLM_APP_0002](../../../vendor/flame/docs/adr/application/FLM_APP_0002__shell_script.md))

## 決定

[FLM_FEA_0004](../../../vendor/flame/docs/adr/feature/FLM_FEA_0004__release_policy.md) の base policy に従いつつ、 flame self では以下の具体選択を採る。

### 配布対象の具体

- **tool 系**: `cli` module の `flame_tool` (= `flame` コマンド) を始めとする `_tool` suffix 付き main package
- **library 系**: `lib` module を始めとする `lib` / `_lib` suffix を持つ Go module

### 実体層 workflow

- 系統ごとに独立した実体層 workflow を持つ ([FLM_FEA_0004](../../../vendor/flame/docs/adr/feature/FLM_FEA_0004__release_policy.md) §リリース起動契機)
  - tool 系: `wf__deploy_tool.yaml`
  - library 系: `wf__deploy_lib.yaml`
- 上位 fan-out 実体層 `wf__deploy.yaml` が両者を再利用可能呼出 ([FLM_ENG_0003](../../../vendor/flame/docs/adr/engineering/FLM_ENG_0003__github_actions.md) §ワークフローの分離 §実体層) で並列起動する。 トリガー層は 1 ファイル 1 ジョブのまま `wf__deploy.yaml` を呼ぶ

### flame CLI の release 関連 subcommand

- library 系の Go 公開 API spec 抽出 ([FLM_FEA_0004](../../../vendor/flame/docs/adr/feature/FLM_FEA_0004__release_policy.md) §版番号の決定経路): `flame ci release spec lib <module-path>`
- tool 系の release 実体: `flame ci release tool`
- library 系の release 実体: `flame ci release lib`
- いずれも flame CLI ([FLI_FEA_0002](FLI_FEA_0002__flame_cli.md)) の責務カテゴリ「CI 補助」 配下に置く。 spec 抽出の実装は Go 標準ライブラリ (`go/parser` / `go/ast` / `go/token` 等) で完結させ、 3rd party 依存は採用しない

### install スクリプトの配置

- 配布対象アプリケーションごとに install スクリプトを当該 module 内 `<module>/scripts/install.sh` に置く。 flame CLI 自身については `cli/scripts/install.sh` が SoT
- private リポジトリ配布における release asset の取得は GitHub API の asset id 経由 (`Accept: application/octet-stream`) で行う。 認証 token は `GITHUB_TOKEN` env もしくは `gh auth token` のいずれかで解決する
- インストール先 default は `$HOME/.local/bin`、 環境変数 `FLAME_INSTALL_DIR` で override 可能

### dry_run の実装

- release ワークフローの dry_run ([FLM_FEA_0004](../../../vendor/flame/docs/adr/feature/FLM_FEA_0004__release_policy.md) §リリースノート 末尾の dry-run モード規約) は同じ実体層 workflow (`wf__deploy_tool.yaml` / `wf__deploy_lib.yaml`) に `dry_run` / `prior_tag_override` 入力を持たせ、 `workflow_dispatch` 経由で feature branch から起動できる形に統合する
- `prior_tag_override` は前回 tag を任意の値に固定して bump 判定経路を再現するための入力で、 dry_run 検証時に release notes 生成の系譜境界を任意の地点に切り替えるために用いる

## 影響

- flame self の release は main マージ自動で `wf__deploy.yaml` が起動し、 配下の `wf__deploy_tool.yaml` / `wf__deploy_lib.yaml` が並列で各配布対象を release する
- flame CLI の release plumbing 関連 subcommand (`flame ci release ...`) は flame CLI 自身に閉じる。 配布対象 library module 側に spec emission の実装は持たない
- private リポジトリ配下での flame バイナリ install は `cli/scripts/install.sh` が `curl ... | bash` 形式で起動でき、 token を解決して asset id 経由で download する
- release 経路を main マージ前に検証する手段として、 `wf__deploy_tool.yaml` / `wf__deploy_lib.yaml` を `workflow_dispatch` + `dry_run` / `prior_tag_override` で feature branch から起動できる。 別系統の検証用 workflow を保守する必要は無い
- flame self の `lib` module は library 配布対象でかつ `lib/cmd/<app>_tool/` を持ちうるため、 同 module を入力にした tool 系 / library 系の 2 release が並列で走るが、 tag 名前空間 (`<app_name>/...` と `<module_path>/...`) が分離するため衝突しない
- 「決定」 で抽象 policy として残した具体実装 (アーカイブ命名フォーマット文字列、 配布対象 OS / arch の個別組合せ、 リリースノート markdown のセクション見出し等) は release ワークフロー定義 / install スクリプト本体 / 関連 module 内ドキュメントに分散して保持される。 これらは仕様変更時に各実装側で更新する

## 評価

代替案として以下を検討した。

### dry_run の実装経路

- **別系統の検証用 workflow (`wf__test_release_notes.yaml`) + 専用 wrapper script を新設する**: production の `wf__deploy_tool.yaml` から独立した経路で release notes 生成だけを試せる。 一方、 (1) 新設 workflow は default branch に存在しないと `workflow_dispatch` で起動できない GitHub Actions の制約により feature branch 上での検証経路として実用的でない、 (2) production と別 code path を検証するため「production と同じ経路を踏んでいるか」 が保証できない、 という不利益がある。 既存の `wf__deploy_tool.yaml` 自身に `dry_run` / `prior_tag_override` 入力を持たせ、 同じ workflow が tag push / release 作成だけを skip する形に統合する方を採用した。

### library 系 spec 抽出の subcommand 階層

- **flame CLI の隠し subcommand (例: `__lib-spec`) として実装する**: 公開 surface に release plumbing 関連の subcommand が並ばない利点があるが、 (1) hidden 化はユーザ / AI への発見性を下げる、 (2) 「全 CLI が共有する generic なシステムコマンド ([FLM_APP_0008](../../../vendor/flame/docs/adr/application/FLM_APP_0008__cli.md) §公開 surface 抽出経路 の `__spec`)」 と「flame 固有の hidden subcommand」 の責務分担が ADR 上曖昧になる、 (3) `flame ci release spec lib` のような階層は CLI 上で release plumbing の場所を表現的に伝えられる、 という事情がある。 公開 subcommand として §subcommand package の階層 ([FLM_APP_0008](../../../vendor/flame/docs/adr/application/FLM_APP_0008__cli.md)) に従い flame CLI 内に階層配置する方を採用した。

### library 系 spec 生成ライブラリの選択

- **`gorelease` / `apidiff` (`golang.org/x/exp` 配下) を採用する**: 公式相当の精度が得られる。 一方、 (1) `x/exp` は実験的位置付けで API 変動リスクがある、 (2) tool 系の CLI spec diff と library 系の Go API spec diff を同型のロジック (flat key-value 比較) で扱いたい、 という事情がある。 自前の AST 走査で flat key-value spec を emit する方を採用した。

### install スクリプトの配置 (代替案)

- **リポジトリルートに置く (`scripts/install-flame.sh` 等) / リポジトリルートに集約し `<app_name>` を引数で渡す**: アプリケーションが 1 つの間は単純だが、 複数アプリに拡張するとリポジトリルートに `install-<app>.sh` が並んで配置の責務が散る。 また共通スクリプト案では認証・OS 検出・PATH 案内のロジックが多 app を考慮した条件分岐で膨らむ。 アプリケーションごとに当該 module 内 (`<module>/scripts/install.sh`) に置く方を採用した。
- **install 先 default を `/usr/local/bin` にする**: PATH に最初から含まれることが多い利点があるが、 sudo 必須となり curl pipe install のシンプルさが崩れる。 ユーザ書き込み可能な `$HOME/.local/bin` を default にし、 環境変数 `FLAME_INSTALL_DIR` で上書きできる形を採用した。

### 実体層 workflow 構成

- **tool / library を 1 ファイル (`wf__deploy_tool_and_lib.yaml`) に統合する**: workflow ファイル数を抑えられるが、 (1) tool は OS / arch matrix の cross-compile を伴い library は伴わないなど jobs の構造 / matrix shape が系統で根本的に異なる、 (2) [FLM_ENG_0003](../../../vendor/flame/docs/adr/engineering/FLM_ENG_0003__github_actions.md) §ワークフローの分離 §実体層 が target を file 名で表現する規約のため target が 2 軸あれば file も 2 つに分けるのが整合的、 (3) 1 系統で 0 件のとき統合 workflow だと無関係な job 構造をスキップする条件分岐が肥大化する、 という不利益がある。 系統ごとに `wf__deploy_tool.yaml` / `wf__deploy_lib.yaml` を独立させ、 上位 fan-out 実体層 `wf__deploy.yaml` で並列起動する方を採用した。
- **fan-out 実体層 `wf__deploy.yaml` を持たず、 tool / library それぞれに独立したトリガー層 (`trg__push__main.yaml` / `trg__push__main_lib.yaml` 等) を持つ**: fan-out 階層を 1 つ削れる。 一方、 [FLM_ENG_0003](../../../vendor/flame/docs/adr/engineering/FLM_ENG_0003__github_actions.md) §トリガー層 で定める「discriminator は当該 event 内の挙動を一意に区別する snake_case」 規約のもと、 push 系トリガーでは discriminator が branch / tag filter と整合する必要があり、 同一 event (push to main) を契機とする 2 トリガーは discriminator が衝突する。 fan-out を実体層 (1 ファイル 1 ジョブ規約の対象外) に置き、 トリガー層は 1 ファイル 1 ジョブのまま `wf__deploy.yaml` を呼ぶ方を採用した。

### 過去経緯

過去に採用していた決定として以下の経緯がある。

- 当初は release notes の PR 列挙を GitHub Search API (`gh pr list --label <name> --search "merged:>=<published_at>"`) で実装していた。 main マージ直後の自動 release で merge から `gh pr list` 実行まで 163 秒経過したケース (PR #22 → flame v1.0.3) でも search index への反映が間に合わず、 PR が release notes から取りこぼされた。 immediate consistent な REST endpoint (compare で commit 集合を確定 → 各 commit の `/commits/{sha}/pulls` で PR を解決 → label で client 側絞り込み) に切替えた。
- 上記の REST 切替え版でも、 PR ↔ commit の reverse-lookup endpoint (`/commits/{sha}/pulls`) が同様に eventual consistent な index に依存することが判明 (PR #23 → flame v1.0.4 で merge から 152 秒経過時点でも reverse lookup に反映されておらず、 release notes の Changes セクションが空になった)。 commit message に GitHub が default で付与する `(#<number>)` を抽出して `/pulls/<number>` (immediate consistent な PR detail endpoint) で解決する経路に再切替えし、 `/commits/{sha}/pulls` への依存を撤去した ([FLM_FEA_0004](../../../vendor/flame/docs/adr/feature/FLM_FEA_0004__release_policy.md) §リリースノート 内の PR 解決規約はこの経緯から導出されたもの)。
- 同時期、 release 経路を main マージ前に検証する手段として、 当初は別系統の workflow (`wf__test_release_notes.yaml`) + 専用 wrapper script を新設する形を採っていた。 上記「dry_run の実装経路」 で述べた制約 (default branch 不在問題 / production と別 code path 問題) により、 既存の `wf__deploy_tool.yaml` 自身に `dry_run` / `prior_tag_override` 入力を持たせる形に統合した。
- 当初は「main マージごとに必ず PATCH が上がる」 挙動で、 module 内に 1 件も変更が無い main マージでも全配布対象の release が走っていた。 結果として release notes が `module/<name>` label PR 0 件で「変更無し」 と表示される release が連発した (例: lib v1.0.16 / lib v1.0.17 が `module/lib` label PR 0 件で生成された)。 [FLM_FEA_0004](../../../vendor/flame/docs/adr/feature/FLM_FEA_0004__release_policy.md) §リリース起動契機 の「前回 release tag → 今回 commit で当該 module ディレクトリ配下のファイル変更が 0 件なら release を生成しない」 判定はこの運用経緯から追加された。 判定経路は path ベース (gh compare API の `.files[].filename` を当該 module dir の prefix で絞り込み) を採る。 label PR 件数で判定する案も検討したが、 fork PR / label 付与前 merge / merge-commit などで label が付かない経路で誤検知 (= 実変更があるのに skip) を起こしやすい一方、 path ベースは GitHub の compare API の生 file 集合に直結するためこれらの取りこぼしが無い。
- 当初は配布対象を tool 系 (`_tool` suffix 付き main package) のみとし、 公開 surface 判定も CLI 表面に固定して library 用途は前提にしていなかった。 [FLM_APP_0007](../../../vendor/flame/docs/adr/application/FLM_APP_0007__go.md) §配置 の module 命名規約に library 配布対象 marker (`lib` / `_lib` suffix) を追加し、 配布対象 library module を release 対象に加える際、 spec 抽出責務は library module 側ではなく flame CLI の `flame ci release spec lib` subcommand に集中させ、 実体層 workflow も `wf__deploy_tool.yaml` / `wf__deploy_lib.yaml` の 2 系統に分割した。 既存 tool 系の起動契機・tag 命名・asset 形式・リリースノート規約は不変に保ち、 library 系を独立した経路として並列起動する形に拡張した。
