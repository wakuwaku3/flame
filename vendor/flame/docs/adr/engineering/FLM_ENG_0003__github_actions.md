# GitHub Actions ワークフローによる CI 検査の整備

## 背景

- flame の品質保証は (1) AI ターン終端 hook、(2) CI、(3) 監視の 3 層で構成し、CI は AI を介さない直接編集 / hook がバイパスされたケース / AI のハルシネーションに備える「最後の砦」と位置付けている ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md))
- hook で実装可能な検査も CI 側で重複実行する方針である ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md))
- flame の hook 層では `scripts/check.sh` が静的検査を起動し、続けて AI レビュー段 (`general-practices-reviewer` / `rule-adr-sync-reviewer` / `adr-reviewer`) を Claude Code の Stop hook 内で起動する ([FLM_ENG_0001](FLM_ENG_0001__claude_code.md))
- AI レビューは AI が応答を返す前に同一ターン内で fix サイクルを回せる位置で実行される必要がある ([FLM_ENG_0001](FLM_ENG_0001__claude_code.md))
- flame のソースコードは GitHub に置かれており、GitHub は GitHub Actions という CI 機構を提供する
- GitHub Actions はリポジトリの `.github/workflows/` 配下に置かれた YAML ファイルを workflow として読み取り、各種イベントを起点に実行する
- GitHub Actions は workflow ファイルの拡張子として `.yml` / `.yaml` の両方を読み込む (片方を要求するわけではない)
- GitHub Actions のイベントには複数の活性化方式がある。`pull_request` は PR head ブランチ側の workflow 定義を使い、`pull_request_target` は base ブランチ側の workflow 定義を使う
- GitHub Actions は再利用可能 workflow 機構 (`workflow_call`) と手動実行機構 (`workflow_dispatch`) を提供する
- GitHub Actions は `${{ }}` 式を `run` ブロック内へ展開する仕様であり、外部入力 (PR head の SHA、PR タイトル等の untrusted 由来データ) を `run` 本文へ直接埋め込むと shell injection 経路となる
- GitHub Actions は matrix strategy によるジョブ並列展開機構と、1 ジョブの失敗で他ジョブを打ち切るかを切り替える `fail-fast` オプションを提供する
- GitHub Actions ワークフロー専用の lint ツール (actionlint 等) およびローカルでイベント発火を再現するツール (act 等) が CLI として広く普及しており devbox から導入できる ([FLM_ENG_0002](FLM_ENG_0002__devbox.md))
- `actions/checkout` action は clone 方法を決めるパラメータとして `fetch-depth` (`0` = 全履歴 fetch) と `ref:` (取得対象 ref の指定) を持つ
- GitHub の PR は server-side で `refs/pull/<PR 番号>/head` という read-only の ref として参照でき、`actions/checkout` の `ref:` に指定して PR head のみを最小 fetch できる
- GitHub Actions の step output は `$GITHUB_OUTPUT` 環境変数が指す一時ファイルへ追記する仕様であり、追記内容は console (CI ログ) には出力されない
- GitHub Actions UI / 通知 / 監査ログ上で job を特定する表示名は `name:` 指定が無い場合に job ID へフォールバックし、ネストした reusable workflow 階層では同名 job が複数表示されやすい
- yq は CLI ベースの YAML プロセッサとして広く普及しており devbox から導入できる ([FLM_ENG_0002](FLM_ENG_0002__devbox.md))
- flame は YAML ファイル全般のルールを定めている ([FLM_APP_0004](../application/FLM_APP_0004__yaml.md))
- flame は shell スクリプト全般のルールを定めている ([FLM_APP_0002](../application/FLM_APP_0002__shell_script.md))
- flame はドキュメント・コメントの自然言語規約を定めている ([FLM_APP_0001](../application/FLM_APP_0001__document.md))
- flame は静的にチェックできるルールを静的チェックで担保する方針である ([FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md))
- flame ではコンテンツ種別ごとに「作成 skill / lint / build / test / ADR ルール検査 skill」の 5 項目を整備する規約がある ([FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md))

## 決定

flame では GitHub Actions ワークフローを以下のルールで扱う。GitHub Actions ワークフローは YAML ファイルの下位種別であり、YAML 全般のルール ([FLM_APP_0004](../application/FLM_APP_0004__yaml.md)) を継承する。ワークフロー内 / 切り出されたシェル本体には shell スクリプト全般のルール ([FLM_APP_0002](../application/FLM_APP_0002__shell_script.md))、ワークフロー内コメントにはドキュメント・コメントの言語規約 ([FLM_APP_0001](../application/FLM_APP_0001__document.md)) を継承する。

### CI で実行する検査の範囲

- [FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md) で hook 層と重複実行すると定めた静的検査は、CI 側でも全種別について実行する
- AI レビュー段は Stop hook 側に固定し CI に移植しない (AI が同一ターン内で fix するループを成立させるため)
- 重い検査 (E2E、複数環境ビルド等) を CI に追加するかは本 ADR の対象外であり、必要時に別 ADR で扱う

### ワークフローファイルの拡張子

- 拡張子は `.yaml` を採用する ([FLM_APP_0004](../application/FLM_APP_0004__yaml.md) を継承)

### ワークフローの分離

ワークフローファイルは「トリガー層」と「実体層」の 2 種類に分離する。1 つのワークフローファイルは必ずどちらか一方として書く。

#### トリガー層

- 役割: 特定の GitHub Actions イベントを受け、単一の実体層ワークフローへの delegation のみを担う薄い層
- 命名: `trg__{event}__{discriminator}.yaml`
  - `event`: GitHub Actions の event 名
  - `discriminator`: 当該 event 内の挙動を一意に区別する snake_case 文字列。意味は event 種別ごとに異なる (activity types を持つ event では types を `_` 連結、branch / tag filter を持つ event では filter 対象を表す snake_case、周期実行ではスケジュール識別子、その他では起動元・用途を表す snake_case)
- flame harness 等の外部 harness が提供する trigger workflow は、 install 時に `<harness-prefix>-trg__{event}__{discriminator}.yaml` 形式で repo の `.github/workflows/` に配置される ([FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §workflow の install 命名規約)。 例: flame の場合 `flame-trg__push__main.yaml` / `flame-trg__pull_request__opened_synchronize_reopened.yaml`。 これは利用側 repo で独自 trigger workflow と命名衝突しないようにするため
- 検査ロジック (実行ステップ・コマンド・実行環境指定・ジョブ間順序付け) を自身に書かない
- 1 ファイル 1 ジョブとし、ジョブの内容は単一の実体層ワークフローへの呼び出しに限定する
- 他リポジトリ / 他ワークフローからの再利用は前提としない (各リポジトリでイベント仕様に合わせて配置する)

#### 実体層

- 役割: 検査・ビルド・デプロイ等の実体ロジックを持つ再利用可能ワークフロー
- 命名: `wf__{verb}.yaml` または `wf__{verb}__{target}.yaml`
  - `verb`: 動作を表す動詞 (`check`, `build`, `deploy` 等)
  - `target`: 動作対象を表す snake_case (動詞単独で意味が成立する場合は省略可)
- 必須要素: 再利用可能呼出と手動実行の両エントリポイントを併設する
- 入力契約: 再利用可能呼出側と手動実行側の入力パラメータは同名同型に揃える
- イベントへの非依存: 実体層は個別イベントに依存させず、イベント差分はトリガー層で吸収する
- 実体層同士の合成: 実体層が他の実体層を再利用可能呼出として呼ぶことを許容する。階層の深さに上限は置かない
- 実体層を呼ぶ `uses:` は absolute 参照 (`uses: <owner>/<repo>/.github/workflows/<file>.yaml@<ref>`) で書く。 caller-relative `uses: ./...` は禁止 (reusable workflow として外部から呼ばれた場合に caller 側 repo の同名 workflow が呼ばれ、 実体層連鎖が壊れるため)。 `<ref>` は git tag (利用側) または `main` (リポジトリ自身の dogfooding) を指定する
- 実体層は caller 側の作業ツリー (caller の `actions/checkout` 結果) に依存する処理を持たない。 実体層内部で必要な scripts / sources は当該 reusable workflow の repo から `actions/checkout` で別 path (`path: .<name>`) に取得する

### ジョブの命名

- すべての job は GitHub Actions UI / 通知 / 監査ログ上で globally-unique な表示名を持つ
- 各 job は `name:` を必ず明示する。GitHub Actions のデフォルト (job ID 由来) に任せない
- `name:` には当該ワークフローファイルのファイル名 (拡張子を除いたステム = `trg__*` / `wf__*` を含む完全名) を含める
- matrix 等で 1 つの job 定義から複数の job が並列展開される場合は、当該展開固有の差別化文字 (matrix variable 値など) も `name:` に含めて並列展開された job 同士の衝突を避ける

### 検査対象ファイルの決定と clone 方針

- 検査対象は base SHA と head SHA を固定値として与え、3 ドット diff (`base...head`) によって抽出する
- ref 名・暗黙の解決 (現在の HEAD と main の merge-base 等) は受けない
- 削除ファイルは検査対象から除外する
- リポジトリの clone は最小限とする。全履歴 clone は使わず、PR head は当該 PR の固有 read-only ref 経由で取得する。マージベース計算等で base SHA が必要な場合は base SHA 単位で個別 fetch する

### shell injection 防御

- 外部入力 (PR head の SHA・PR タイトル等の untrusted 由来データ) を shell コマンドへ直接展開しない
- 必ず変数経由で受け渡し、shell 展開時には引用符で保護する

### inline shell の制限

- ワークフロー YAML 内の inline shell には非自明な shell ロジックを置かない
- shell ロジックの本体は別ファイルに切り出し、`.github/scripts/` 配下に配置する。当該ファイルは shell スクリプト全般のルール ([FLM_APP_0002](../application/FLM_APP_0002__shell_script.md)) を継承する
- ワークフロー YAML 内の inline shell は環境変数設定と切り出し済みスクリプトの呼び出しに限定する

### CI step 出力の可観測性

- step が後段に値を渡す際は、値の引き渡しと CI ログへの複写を同時に行う
- ログに残らない引き渡し経路 (書き込み先のみ) は使わない

### 並列化

- 検査 CI はコンテンツ種別を単位として job を分割し並列実行する
- 1 種別の job 失敗で他種別の job を打ち切らない
- 変更ファイル 0 件のときも空起動せず、必ず 1 件以上の success job を返す

### 静的検査の経路と共通化

- 静的検査の検査単位 (checker) と、hook 層 / CI 層との共通化境界 (種別判定 / checker / 実行環境を共有する範囲) は [FLM_FEA_0001](../feature/FLM_FEA_0001__checker.md) に従う

### test の必須化と配置

- 各ワークフローファイル (`trg__*.yaml` / `wf__*.yaml`) は対応する **test script** を 1 本必ず持つ
- test script の配置は `.github/workflows/tests/{workflow-stem}.sh` (workflow と同 dir 配下の `tests/` サブディレクトリ、 拡張子を除いた basename を共有)
- harness が提供する trigger workflow に対応する test script も、 install 時に `<harness-prefix>-<workflow-stem>.sh` 形式で配置される ([FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §workflow の install 命名規約)。 例: flame の場合 `tests/flame-trg__push__main.sh`
- 共有アサーションヘルパは `.github/workflows/tests/shared/` 配下に置く
- test script は対応 1 ワークフローに対する以下の dispatch / parse 検証を、 自身が必要とする観点のみ (= no-op ヘルパは省略可) で組み立てる:
  - 当該ワークフロー内の `jobs.<id>.uses` が指す repo-local の実体層が実在すること (typo / 参照切れ検出)
  - 当該ワークフロー内の `jobs.<id>.with` のキー集合が呼出先の `workflow_call.inputs` と整合 (required ⊆ with ⊆ 宣言済みすべて) すること
  - 当該ワークフローが inline `run:` から参照する `.github/scripts/<rel>.sh` が実在すること
  - トリガー層に限り、 当該イベントの合成 payload を渡した `act --list` で当該ワークフローが parse できること (docker 必須、 docker 不在時は SKIP 扱い)
- lint と test は同一 entrypoint (`flame check github-actions`) に束ねる。 lint は当該 test script の実在を必須化し、 続けて当該 test script を子プロセスとして実行する。 lint pass と test pass は同じ起動経路で同時に保証される

### 5 項目の整備状況

[FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md) で定める 5 項目について以下を整備する。

| 項目 | 整備方針 |
| --- | --- |
| 作成 skill | 整備 (`.claude/skills/github-actions/` でワークフロー新規作成時の命名規約・必須要素・lint / トリガー発火検証手順・対応 test script の同時生成までを完了させる skill を整備) |
| lint | 整備 (`flame check github-actions` から GitHub Actions 専用 workflow lint ツールおよびファイル名規約・必須要素・トリガー層キー集合制約のカスタム静的検査を実行。 §test の test script 実在検証も lint 段に含める) |
| build | 省略 (静的設定のため出力生成の概念がない) |
| test | 整備 (各ワークフローファイル `<basename>.yaml` に対応する `.github/workflows/tests/<stem>.sh` で、 当該ワークフロー単体の dispatch / parse 検証を行う。 lint 段から `flame check github-actions` 経由で動的に discover / 実行する) |
| ADR ルール検査 skill | 省略 (lint で完結する範囲に限定) |

## 影響

- 検査 CI は PR の opened / synchronize / reopened を起点として走り、変更ファイルに対してのみ各 checker が実行される
- 同じ静的検査が hook と CI で重複実行されるため、hook 層で fix できなかった違反は CI でも同一の違反として再検出される
- AI レビュー段は CI に移植されないため、CI で fail する違反は静的検査由来のものに限られる
- ワークフロー検査ツール (例: actionlint) とトリガー発火検証ツール (例: act) を devbox に追加する必要があり、`devbox.json` / `devbox.lock` のメンテナンス対象が増える ([FLM_ENG_0002](FLM_ENG_0002__devbox.md))
- カスタム静的検査スクリプト (ファイル名規約・キー集合制約・トリガー層と実体層の対応関係等) の保守コストが発生する
- ワークフローファイル名 (`trg__*` / `wf__*`) から、イベント受信層 / 実体ロジック層の区別と、event 種別および discriminator が機械的に判別できる
- トリガー層の薄層制約は GitHub Actions YAML スキーマの以下のキー集合で静的検査により強制される: 許可キー = `name` / `on` / `jobs.<id>.name` / `jobs.<id>.uses` / `jobs.<id>.with` / `jobs.<id>.secrets` / `jobs.<id>.permissions`、明示的禁止キー = `jobs.<id>.steps` / `jobs.<id>.run` / `jobs.<id>.runs-on` / `jobs.<id>.needs`。許可キー以外の top-level キー (`env` / `defaults` / `concurrency` 等) および許可キー以外の job-level キーも exhaustive に reject される。これによりトリガー層に検査ロジックを書き始める誘惑が静的検査で抑止される
- 実体層の両エントリ併設要件は GitHub Actions では `on.workflow_call` と `on.workflow_dispatch` の併設、入力契約は `workflow_call.inputs` と `workflow_dispatch.inputs` の同名同型として実装される。両者を併設するため、CI 経由・手動経由の両方から同じ実体ロジックを再現でき、failure 時のローカル / GitHub UI 上の手動再現経路を持てる
- 実体層のイベント非依存要件は GitHub Actions では「個別イベント (`pull_request` / `push` / `schedule` 等) を実体層の `on` に直接書かない」という具体形に落ちる。これにより別イベントからも同じ実体層を再利用できる
- 実体層同士の合成を許容するため、合成階層が深くなった場合に呼び出し関係を追跡するコストが発生する
- 実体層を呼ぶ `uses:` は absolute 参照に統一されるため、 リポジトリ自身の dogfooding でも実体層連鎖は外部参照 (`@main` 等) で記述され、 利用側 caller と同じ経路で動作する。 別経路 (caller-relative `./`) を持たない分、 利用側で発生する不具合 (例: 実体層連鎖の参照解決問題) を開発時に再現できる
- 実体層は caller の作業ツリー非依存となるため、 利用側 repo に当該 workflow を提供する repo の source を持たせなくても reusable workflow として動作する。 必要な tool / script は実体層内部で self-checkout (`actions/checkout` 別 path) 経由で取得する経路に統一される
- 3 ドット diff によりマージベースからの差分が取れるため、main 側の進行による偽陽性 (PR が触っていないファイルが diff に含まれる) を避けられる
- 削除ファイルが検査対象外のため、削除によって発生する参照切れ (Markdown リンク切れ等) は本 ADR の検査では検出されない
- shell injection 防御は GitHub Actions では「外部入力を `env:` で受けて `run` 内では引用付き変数展開を使う」「`${{ ... }}` 展開を `run` 本文に直接埋め込まない」という具体形に落ちる。これにより shell injection 経路が静的検査で塞がれる
- `pull_request` を採用するため、PR head の workflow 定義・scripts・devbox manifest 等がそのまま CI runner 上で実行される。base/head の trust 境界を CI 実装側 (workflow / 切り出しスクリプト / devbox manifest 等の base 由来 overlay) で背負う必要がなくなり、信頼分割プラミング (`.github/scripts/overlay-base-trusted.sh` 等) を撤去できる。これにより 3 層 FB ループ ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md)) の 2 層目 (CI) を編集する際の認知コストが下がり、checker 追加・改修時に「base / head どちらの SHA で動くべきか」を毎回意識する必要がなくなる
- 並列化方針は GitHub Actions の matrix strategy で `fail-fast: false` 相当に固定される。種別ごとに job が分割され早期打ち切りが起きないため、AI / 人間が 1 PR 中の全違反を 1 回の CI 実行で把握できる
- 変更ファイル 0 件のときも success job を返すため、branch protection の required status check が「未実行」で待ち続ける事故を避けられる
- 最小 clone 方針は GitHub Actions では `actions/checkout` で `ref: refs/pull/<n>/head` 指定 + 必要に応じた base SHA の depth=1 個別 fetch という具体形に落ちる。fetch サイズが小さくなり CI 実行時間・転送量・GitHub Actions cache 消費が減るが、3 ドット diff のためのマージベース計算では base SHA を別ステップで取得する手順を持つ必要がある
- 各 job が `name:` を持ちワークフローファイルのステムを含むため、GitHub Actions UI / 通知 / 監査ログでネストした reusable workflow 階層 (例: trigger → wf__check → wf__check__diff) でも各 job がどの workflow 由来か追跡できる。matrix 展開された job は GitHub Actions では `name:` に matrix 変数 (例: `${{ matrix.checker }}`) を interpolate する具体形 (例: `wf__check / ${{ matrix.checker }}`) で並列展開された job 同士の衝突を回避する
- inline shell の制限は GitHub Actions では `run:` block の制約として実装される。ワークフロー YAML が薄くなり shell 本体は shellcheck で静的検査される。`.github/scripts/` 配下の shell も flame の shell 規約 ([FLM_APP_0002](../application/FLM_APP_0002__shell_script.md)) を継承し、shell スクリプトの配置は `scripts/` (汎用) と `.github/scripts/` (CI 専用) に二分される
- 値引き渡しと CI ログへの複写の同時化は GitHub Actions では「step output 用の `$GITHUB_OUTPUT` 一時ファイルへの書き込みと console への出力を `tee -a "$GITHUB_OUTPUT"` 等で同時に行う」具体形に落ちる。CI ログだけで step output 値が直読でき、debug 時に値を確認するために echo 文を後追いで足したり workflow を再実行したりせずに済む
- ワークフロー / 切り出されたシェルから YAML をパースする必要が生じた場合、CLI ベースの YAML プロセッサ (yq) を採用する。flame は Python を採用言語として持たない (devbox に Python ランタイムを置かない) ため、Python + PyYAML 等の Python 依存ツールチェインは採らない
- §test の必須化により、 ワークフロー追加・改変ごとに対応 test script (`.github/workflows/tests/<stem>.sh`) を 1 本セットで書くコストが発生する。 lint と test を同一 entrypoint に束ねたため、 hook / CI のいずれの起動経路でも「lint pass = 対応 test も走った」 が保証され、 test 追加忘れが lint 違反として顕在化する
- §test の配置は shell スクリプトの配置規約 ([FLM_APP_0002](../application/FLM_APP_0002__shell_script.md) §配置) における 3 番目の許可ロケーションとなる。 test script のファイル名は対応ワークフロー名 (snake_case + ダブルアンダースコア区切り) を継承し、 既存 shell の kebab-case 規約に対する例外を構成する。 例外を許す理由は「ワークフローと test の対応を 1 視で読み取れること」 を §test の lint 検出経路 (= `<dir>/tests/<stem>.sh` の決定論的 path 解決) と整合させるため
- §test 実行は外部バイナリ (yq / jq / act) を要し、 act の動的 parse 検証は docker daemon に依存する。 docker 不在環境 (hook 層の一部 / CI runner の一部) では act 検証だけが SKIP となり、 残る静的アサーション (uses target 実在 / inputs parity / referenced scripts 実在) は引き続き走る
- ワークフロー本体の test 観点は flame の test layer ([FLM_APP_0009](../application/FLM_APP_0009__test.md)) における service-level test に対応する。 ワークフロー = 利用者から見える 1 入口 (CI 起動経路の dispatcher) と捉え、 当該入口の dispatch 正しさを外部観測可能な形 (uses 解決 / inputs 整合 / act parse) で検証する
- flame harness 由来の trigger workflow / test script は install 時に flame-prefix で配置されるため、 利用側 repo の `.github/workflows/` には flame 由来 workflow と利用側独自 workflow が prefix で識別可能な形で並ぶ
- 命名規約 lint (= `trg__*.yaml` / `wf__*.yaml` / `tests/<stem>.sh` 等の存在検査) は flame-prefix 付きファイルも対象として扱う
- 依存側プロジェクトにも本 ADR の規約が伝播する (本 ADR は ENG カテゴリのため)

## 評価

代替案として以下を検討した。

- **`pull_request_target` を起点イベントとする**: `pull_request_target` は base ブランチ側のワークフロー定義を使うため、PR 著者がワークフロー本体を書き換えても CI 実行に影響しない (workflow 定義をリポジトリ管理下で固定できる) 利点がある。一方で PR head の SHA を checkout する以上、検査対象データは head・実行コードは base という分割を CI 実装側 (workflow / 切り出しスクリプト / devbox manifest 等の base 由来 overlay) で背負う必要があり、信頼分割プラミング (`.github/scripts/overlay-base-trusted.sh` 等) の維持コストが恒常的に発生する。flame では 3 層 FB ループ ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md)) の 2 層目 (CI) を頻繁に編集するため信頼分割プラミングが checker 開発の編集コストを支配的に膨らませており、これを撤去することを主因として `pull_request` を採用した。補助的な観察として、`pull_request_target` のデフォルト `GITHUB_TOKEN` は base repo に対する write 権限を持つのに対し、fork からの PR に対する `pull_request` のデフォルト `GITHUB_TOKEN` は read-only に絞られ base repo の secret も注入されないため、trust 境界が無くても base 側への書き込みや secret 取り出しは発生しない (`pull_request_target` を維持しなくても compromise の余地が広がるわけではない)。
- **2 ドット diff (`base..head`) で差分を計算する**: 2 ドットだと head と base の単純差分となり、main 側の進行 (PR が触っていないファイル) が diff に含まれる。3 ドット (`base...head`) はマージベースからの差分となり、PR が実際に変更したファイルのみを得られるため 3 ドットを採用した。
- **削除ファイルも検査対象に含める**: 削除されたファイルに対して静的検査を走らせても (内容が無いため) 意味のある結果は得られない。一方、削除によって発生する参照切れは別の検査 (リンクチェッカー等) で扱うべき問題で、本 ADR の検査対象に混ぜると責務が肥大化する。削除ファイルは対象から除外し、参照切れ検査は必要時に別 ADR で扱う方を採用した。
- **トリガー層と実体層を分離せず 1 ファイルに書く**: イベントごとにロジック全体が複製され、別イベント (`push` / `schedule` 等) から同じロジックを再利用しづらい。また、トリガー層に検査ロジックを書ける状態だと「薄く保つ」というルールが運用で形骸化しやすい。分離 + キー集合制約による静的強制を採用した。
- **トリガー層が複数の実体層を順序づけて呼ぶことを許容する**: 合成の意図がトリガー層に染み出し、トリガー層の責務が「イベント受信」を超える。合成は実体層内で表現することにし、トリガー層は単一実体層への dispatch のみに限定した。
- **実体層に `workflow_dispatch` を必須化しない (`workflow_call` のみ)**: 手動デバッグの起点を持たないと、CI で発生した failure をローカルや GitHub UI 上の手動実行で再現する経路が無くなる。`workflow_call` と `workflow_dispatch` の両方を必須とした。
- **AI レビュー段を CI にも移植する (重複実行)**: AI レビューを CI で fail させる構造は、AI が修正できない位置 (PR レビュー段階) で違反が顕在化することを意味する。AI レビューは AI 開発ループ内で「次の応答を返す前に AI 自身が直す」ことに価値があり、CI に移植すると AI が PR の round trip を経ないと fix できない。AI レビューは Stop hook 側に固定する方を採用した。
- **トリガー層 / 実体層の prefix を `trg-` / `wf-` (ハイフン区切り) にする**: YAML ファイルの命名として ADR / flow ドキュメント等の snake_case 規約 ([FLM_GEN_0001](../general/FLM_GEN_0001__adr.md) / [FLM_APP_0006](../application/FLM_APP_0006__flow_document.md)) と整合しない。`trg__` / `wf__` (アンダースコア区切り、prefix とタイトルの境界をダブルアンダースコアで表現) を採用した。
- **discriminator を「activity types を全列挙する」に固定する**: types 数の多い event ではファイル名が長大になる (例: `trg__pull_request__opened_synchronize_reopened.yaml`)。本 ADR では discriminator の意味論を「当該 event 内の挙動を一意に区別する snake_case 文字列」と規定するに留め、長過ぎる場合の選択肢として「複合させず複数ファイルに分ける」「types 不問の汎用名 (例: `trg__pull_request__all.yaml`) にする」を残した。
- **複合 discriminator (1 つの event に対し activity types と branch filter の両方で区別したいケース) を全面禁止する**: event 仕様によっては複合せざるを得ないケースが想定される。本 ADR では複合を許容しつつ、命名揺れの源泉となるのを防ぐため、複合する場合は「意味のある語にまとめる」または「複合させず別ファイルに分ける」のいずれかを設計時に選ぶ方針とした。
- **検査単位のジョブ分割で `fail-fast: true` を採用する**: 早く fail させてリソースを節約する利点はあるが、AI / 人間が 1 PR 中の全違反を把握するには再実行が必要となり、修正サイクルが長くなる。`fail-fast` を無効化する方を採用した。
- **`fetch-depth: 0` で全履歴 clone する**: マージベース計算が単一 fetch で完結する利点はあるが、リポジトリ規模に比例して fetch サイズ・CI 実行時間・GitHub Actions cache 消費が増える。flame では CI レイテンシを 3 層 FB ループ ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md)) の 2 層目として短く保つ必要があり、最小 clone (PR head の固有 ref + 必要に応じた base SHA の個別 fetch) を採用した。
- **step output を書き込み先のみ (console 非複写) で出力する**: 公式ドキュメントの例の多くがこの形だが、CI ログから step output 値が読めず、debug 時に echo 文を後追いで足すか workflow を再実行する必要が生じる。書き込みと console への複写を同時に行う形を必須化する方を採用した。
- **inline shell をワークフロー YAML 内に保持する**: 一見配置が単純化するが、(1) shellcheck の検査対象から外れる、(2) YAML スカラー内の shell quoting が二重 escape を要する、(3) GitHub Actions のログ表示で line number と shell debug 情報が分離する、(4) shell ロジックの reuse が困難、という不利益が大きい。`.github/scripts/` への切り出しを必須化し、ワークフロー YAML は薄い caller に限定する形を採用した。
- **切り出した CI 専用 shell も既存の `scripts/` 配下に集約する**: 全 shell を 1 箇所に置く単純さがある一方、ワークフローからの呼び出し元との物理距離が広がり、CI 専用シェルかそうでないかの区別がディレクトリ構造から失われる。GitHub の慣習に従い `.github/scripts/` を CI 専用シェル置き場として用意する方を採用した (shell スクリプト規約 [FLM_APP_0002](../application/FLM_APP_0002__shell_script.md) 側でも当該配置を許可する形に揃える)。
- **job の `name:` を省略して GitHub Actions のデフォルト (job ID 由来) に任せる**: ネストした reusable workflow run で同名の job (例: `check`) が複数表示され、どの workflow 由来か UI / 通知から判別できなくなる。各 job が `name:` を明示し、ワークフローファイルのステムを `name:` に含める形を必須化する方を採用した。
- **ワークフローでの YAML パースに Python (pyyaml 等) を採用する**: Python 標準ライブラリと連携する自由度が増す一方、flame に Python ランタイムを採用すると devbox の管理対象に Python とそのパッケージが追加され、現在の採用言語 (shell) を超えて言語の組合せが増える。flame の採用言語を抑える方針 (新規言語の追加は別 ADR で明示的に判断する) と、YAML パースが yq 1 ツールで充足する事実から、Python を採用しない方を選んだ。
- **test を 1 本にまとめてワークフロー全件を 1 起動で検査する (旧 `scripts/test-github-actions-trigger.sh` 形式)**: 起動経路が 1 つに集約され dispatch ロジックも単純化される利点はある。 一方、 (1) どのワークフローに対する検査か当該 1 ファイル内の制御フローを追わないと判別できず、 ワークフロー数が増えると 1 ファイルが長大化する、 (2) ワークフローの追加・削除と test の追加・削除が 1:1 対応にならず、 削除忘れ / 追加忘れが lint で検出できない、 (3) 1 ファイルに全ワークフローの synthetic payload と assertion ロジックが詰まるため、 1 ワークフロー固有の検査軸 (例: leaf wf__では act 検証は不要、 fan-out wf__ では assertion を増やす) を表現するための switch 分岐がファイル内に蓄積する、 という不利益が大きい。 ワークフロー 1 本 = test script 1 本の 1:1 対応に分割し、 共有部分は `tests/shared/` に括り出す形を採用した。
- **test script を `scripts/` 配下に置く (汎用 shell と同居)**: 配置規約 ([FLM_APP_0002](../application/FLM_APP_0002__shell_script.md)) との整合は取れるが、 (1) 検査対象 (ワークフロー) と test script の物理距離が広がり、 「どの test がどのワークフローに対応するか」 をファイル名から推測する経路がなくなる、 (2) test script はワークフロー以外を検査せず汎用性も持たないため、 汎用 `scripts/` への混入は責務分散として整合しない、 (3) ワークフローの追加・削除と test script の同時編集を 1 dir 内で完結できなくなる、 という不利益がある。 ワークフローと同 dir 配下の `tests/` サブディレクトリに置き、 ファイル名を workflow stem と一致させる方を採用した。
- **lint と test を別 entrypoint に分ける (旧 `scripts/check-github-actions.sh` + `scripts/test-github-actions-trigger.sh` の 2 entrypoint)**: 各 entrypoint の責務が単純化される利点はある。 一方、 (1) hook / CI から起動する経路を 2 つ維持する必要があり、 「lint だけ走って test が走らない」 状態を作りやすい、 (2) test 不在を lint で検出できないため、 ワークフロー追加時に test 同時追加を強制する経路が無く、 運用で形骸化しやすい、 (3) 経路が 2 つあると static check のジョブ分割 (FLM_FEA_0001 §並列化) でも 2 entry を管理する必要が生じる、 という不利益がある。 lint entrypoint (`flame check github-actions`) に test 実行を内包し、 test 実在検証を lint 違反として顕在化させる方を採用した。
- **test script のファイル名を kebab-case (FLM_APP_0002 §ファイル名 に従う)** にする: shell スクリプト全体の命名規約と整合する利点はある。 一方、 ワークフローのファイル名が snake_case + `__` 区切り (FLM_ENG_0003 §命名) のため、 kebab-case に変換した test script 名から元ワークフロー名を 1 視で復元できず、 「どの test がどのワークフローに対応するか」 が `flame check github-actions` の path 解決ロジックを読まないと分からなくなる。 対応関係を「ファイル名の 1:1 mirror」 として静的に固定する方が読み手に対するシグナルが強く、 lint 側の path 解決もシンプルになるため、 snake_case + `__` 区切りで stem を継承する方を採用した。
- **実体層 → 実体層の `uses: ./...` を許容する**: 同 repo 内で簡潔に書ける利点はあるが、 reusable workflow として外部から呼ばれた場合に caller 側 repo の同名 workflow が呼ばれ、 当該 reusable workflow を提供する repo 側の実体層連鎖が壊れる。 absolute 参照 (`uses: <owner>/<repo>/.github/workflows/<f>.yaml@<ref>`) に統一する方を採用した。 副次的に、 dogfooding でも `@main` 等の絶対参照経路を踏むため、 利用側で発生する不具合を開発時に再現できる利点もある
- **実体層が caller 側の checkout を前提とする (利用側 repo に reusable workflow 提供 repo の source を要求する)**: 実装は単純だが、 利用側 repo にツール提供 repo の source code を持たせる前提となり、 source 配布経路を別途用意する必要がある。 実体層内部で `actions/checkout` を別 path に追加して self-checkout する経路に統一することで、 利用側 caller の作業ツリーを汚染せず、 かつ利用側に source を持たせる前提を排除した
- 当初は flame harness 由来の trigger workflow も `trg__*.yaml` 命名で install していたが、 利用側 repo で独自 trigger workflow と命名衝突する可能性が顕在化した。 [FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §workflow の install 命名規約 で flame-prefix 付き install を規約化したことに伴い、 本 ADR にも対応する命名規約を反映した
