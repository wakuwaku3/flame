# Shell スクリプトの基本ルール

## 背景

- flame では FB ループ ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md)) の hook 層・CI 層、および devbox タスク等から呼び出される自動化処理を shell スクリプトで実装している (例: `scripts/check-adr.sh`、`scripts/check-document.sh`)
- shell スクリプトには静的解析ツール (lint) が存在する
- flame は静的にチェックできるルールを静的チェックで担保する方針である ([FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md))
- shell の慣習として、export して子プロセスへ伝搬させる環境変数は UPPER_SNAKE_CASE で書く文化がある
- POSIX shell および bash では、ファイル内ローカル変数と環境変数の名前空間が同一であり、命名以外で両者を区別する手段はない
- flame ではコンテンツ種別ごとに「作成 skill / lint / build / test / ADR ルール検査 skill」の 5 項目を整備する規約がある ([FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md))
- flame は GitHub Actions ワークフロー内の `run:` block に shell ロジックを書くと shellcheck の検査対象から外れることを背景に、CI 専用シェルを別ファイルへ切り出して `.github/scripts/` 配下に置く方針を採っている ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md))
- shell には heredoc 機構があり、他言語のソース (Python / awk / sed のスクリプト本体等) を文字列として埋め込み外部インタプリタへ渡す書き方が一般的に行われる
- heredoc 埋め込みされた他言語コードは shellcheck からは「文字列の塊」としか見えず、当該言語固有の構文・命名・型エラーは検査されない

## 決定

flame では shell スクリプトを以下のルールで扱う。

### 配置

- shell スクリプトは `scripts/` 配下に配置する
- GitHub Actions ワークフローから呼び出される CI 専用 shell スクリプトは `.github/scripts/` 配下に配置する ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md))
- GitHub Actions ワークフロー本体に対する test script は `.github/workflows/tests/` 配下に配置する ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md) §test の必須化と配置)
- ただし、特定モジュール / 特定コードベースに密結合した shell スクリプトは当該モジュールのディレクトリに配置する

### ファイル名

- 形式: `{kebab-case-name}.sh`
- 拡張子は `.sh` を付ける
- 区切り文字はハイフン (`-`) を用い、アンダースコア (`_`) は使わない
- 例外: `.github/workflows/tests/` 配下に置く workflow 本体に対応する test script は、 対応ワークフローの basename (snake_case + ダブルアンダースコア区切り) を継承する。 `.github/workflows/<basename>.yaml` に対する test script は `.github/workflows/tests/<basename_without_yaml>.sh` となる ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md) §test の必須化と配置)。 同 dir 配下でもワークフローに対応しない補助ファイル (例: `.github/workflows/tests/shared/` 配下の共通ヘルパ) は本例外の対象外で kebab-case に従う

### 変数の命名

- ファイル内で定義するローカル変数は lower_snake_case で書く
- export して子プロセスへ伝搬させる変数、および外部から受け取る環境変数のみ UPPER_SNAKE_CASE を許可する

### 多言語混在の禁止

- shell スクリプトは他言語の処理ロジック本体 (Python / awk / sed 等の多行スクリプト) を heredoc 等で埋め込まない
- 当該言語の処理ロジック本体が必要な場合は、当該言語のソースを別ファイルに切り出し、shell 側からは外部コマンドとして呼び出す形に分離する
- jq の filter 表現や `find -exec` の述語のような、 外部コマンドへ渡す短いクエリ DSL は本ルールの対象外とし、 shell 文字列として inline で持ってよい

### コメント

- shell コメント (`#`) はドキュメント・コメントの自然言語規約 ([FLM_APP_0001](FLM_APP_0001__document.md)) を継承する

### lint

- 全 shell スクリプトは shell lint の検査対象とする
- 検査は flame の FB ループ規約 ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md)) に従い hook と CI の両方で実行する
- リポジトリ全域の shellcheck 設定 (`.shellcheckrc`) に SC2016 (single-quote 文字列内の変数参照記法 `$xxx` を「展開されない shell 変数」 として警告するルール) を `disable=SC2016` で無効化する。 §多言語混在の禁止 で許容した jq filter inline 利用 (例: `jq '.[] | $p'`) では single-quote 内の `$p` は jq 変数であり shell 変数ではないため、 single-quote で括ることが正しい挙動 (= shell 展開を起こさない) になる。 SC2016 は flame で構造的 false positive となるため、 [FLM_GEN_0006](../general/FLM_GEN_0006__no_lint_suppression.md) §グローバル無効化 経路に沿って設定ファイル側で 1 件無効化する

### 5 項目の整備状況

[FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md) で定める 5 項目について以下を整備する。

| 項目 | 整備方針 |
| --- | --- |
| 作成 skill | 整備 (`.claude/skills/shell-script/` で動作確認まで完了させる skill を整備) |
| lint | 整備 (shell lint を hook / CI で実行) |
| build | 省略 (スクリプト自体が成果物のため出力生成の概念がない) |
| test | 省略 (作成 skill 内の動作確認で代替し、テストフレームワーク (bats 等) は現時点では採用していない) |
| ADR ルール検査 skill | 省略 (lint と一般プラクティス観点でカバー) |

## 影響

- shell スクリプトの配置場所が `scripts/` (または当該モジュールのディレクトリ) に固定される
- shell lint (例: shellcheck) の設定・維持コストが発生する
- shell lint の指摘を局所的に抑制するための注釈が必要になる場合がある
- ファイル名のハイフン区切りにより、shell スクリプト名と shell の関数名 (関数名にハイフンは使えない) で命名規則が分かれる
- ローカル変数を lower_snake_case に揃えることで、export 対象の環境変数 (UPPER_SNAKE_CASE) と読み手が一目で区別できる
- 既存の shell スクリプトに UPPER_SNAKE_CASE のローカル変数が含まれている場合、リネームが必要になる
- shell lint によって未引用変数・未使用変数・配列展開ミス等が静的に検出される
- 命名規約とファイル名規約が ADR で固定されるため、AI が新規 shell スクリプトを生成する際にも本規約に従う圧力が働く
- 他言語の処理ロジック本体の埋め込み禁止により、当該本体が必要な箇所では独立ファイル + 外部コマンド呼び出しとなり、各ファイルが当該言語の lint / formatter で個別に検査される
- 多言語連携のうち処理ロジック本体は「shell + 別ファイル」の組合せに制限されるため、shell 内に多言語コードが密結合した結果として shellcheck の検査盲点が拡大する事態を防げる
- jq filter や `find -exec` 述語のような短いクエリ DSL は shell 文字列として inline で持てるため、 1〜数行の引数 DSL のために別ファイル + 参照経路を増やすコストは発生しない
- CI 専用シェルが `.github/scripts/` に配置されるため、汎用シェル (`scripts/`) と CI 由来シェル (`.github/scripts/`) がディレクトリで分離され、呼び出し関係 (workflow → CI 専用シェル) と汎用呼び出し (hook / 手動 / devbox task → 汎用シェル) の境界が明確になる

## 評価

代替案として以下を検討した。

- **ファイル名を snake_case にする**: ADR 等の他コンテンツで snake_case を採用しているため統一感はあるが、shell スクリプト名は CLI として実行されるコマンドであり、kebab-case が広く慣習として用いられている。CLI 命名の慣習に揃える方を採用した。
- **ファイル内変数も UPPER_SNAKE_CASE に統一する**: shell の伝統的なスタイルだが、export 対象の環境変数とローカル変数が同じ表記となり、読み手が用途を見分けられない。命名で用途を区別する方を採用した。
- **shell スクリプトを使わず別言語 (Go / TypeScript / Python 等) でラッパーを書く**: 自動化処理が複雑化した場合は別言語を採用する選択肢はあるが、軽量な hook / CI 用ラッパーのために別言語ランタイムを導入する初期コストの方が大きい。簡易な処理は shell に留める。
- **shell lint を導入しない**: AI / 人間のレビュー時にしか問題が検出されず、flame の静的チェック優先方針 ([FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md)) と整合しない。
- **配置場所をディレクトリ単位で固定しない (任意の場所に置く)**: 横断的に呼ばれるスクリプトとモジュール固有のスクリプトが混在し、検索・移動のコストが上がる。横断的なスクリプトは `scripts/` に集約する形を採用した。
- **CI 専用シェルも `scripts/` に集約する**: 全 shell を 1 箇所に置く単純さがある一方、ワークフローからの呼び出し元との物理距離が広がり、CI 専用かそうでないかの区別がディレクトリ構造から失われる。GitHub の慣習に従って `.github/scripts/` を CI 専用シェルの置き場として用意し、汎用 (`scripts/`) と CI 専用 (`.github/scripts/`) を物理的に分ける形を採用した。
- **shell に他言語コード (Python / awk / sed の本体等) を heredoc で埋め込むことを許容する**: 単一ファイルで完結する利点はあるが、(1) 埋め込まれた他言語コードは shellcheck から「文字列の塊」としか見えず構文・命名・型の検査が走らない、(2) shell quoting と他言語 escape の二重 escape を要する、(3) debug 時に line number が混乱する、(4) 他言語コードが膨らむほどファイル全体が複合的になり読みづらくなる、という不利益が大きい。多言語ロジックは別ファイル化と外部コマンド呼び出しに分離する形を採用した。

過去に採用していた決定として以下の経緯がある。

- 当初は配置を `scripts/` 配下 (および特定モジュール内) のみに固定していた。GitHub Actions ワークフロー側で inline shell を切り出して `.github/scripts/` に置く方針を [FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md) で決めたのに伴い、本 ADR の配置に `.github/scripts/` を追加した。
- 当初は shell スクリプト本文に他言語のソース (Python / awk / sed 等) を heredoc で埋め込むことを暗黙に許容していた (明示的な禁止は持たなかった)。埋め込み他言語コードが shellcheck の検査盲点となる構造的問題が判明したため、「多言語混在の禁止」セクションを新設して明示的に禁止する形に変更した。
- 当初は「多言語混在の禁止」を厳格に解釈し、jq filter のような短いクエリ DSL も別ファイル化する運用を採っていた (例: `.github/scripts/deploy/flatten-cli-spec.jq` を切り出して `jq -f` で読み込む)。短いクエリ DSL は他言語の処理ロジック本体ではなく外部コマンドへ渡す引数の一種であり、別ファイル化のコスト (ファイル分散・参照コスト) の方が shellcheck 検査盲点回避のメリットを上回ると判断して、本ルールの対象を「他言語の処理ロジック本体」に限定する形に緩和した。
- 当初はコメントの自然言語規約を本 ADR で持たず、執筆者の任意としていた。[FLM_APP_0001](FLM_APP_0001__document.md) でドキュメント本文・ソースコード内コメントの自然言語規約 (日本語) が定められたのに伴い、本 ADR にも「コメント」セクションを設け [FLM_APP_0001](FLM_APP_0001__document.md) の継承を明示する形に変更した。
