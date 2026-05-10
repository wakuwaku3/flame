# lint の局所抑制を最小化する

## 背景

- flame は静的にチェックできるルールを既存 lint / 静的解析ツールで担保する方針である ([FLM_GEN_0004](FLM_GEN_0004__static_check.md))
- 多くの lint ツールはソース内に「局所抑制コメント」を書ける機構を持つ (例: shellcheck の `# shellcheck disable=SCxxxx`、ESLint の `// eslint-disable-next-line`、markdownlint の `<!-- markdownlint-disable -->`、yamllint の `# yamllint disable-line rule:xxx`、actionlint の `# actionlint disable rulename`)
- ルールをリポジトリ全体で抑制する仕組みも各 lint ツールに備わっている (例: `.shellcheckrc`、`.markdownlint-cli2.yaml`、`.yamllint`、`actionlint.yaml`、`tsconfig.json`、`.eslintrc`)
- flame は AI 開発 harness ([FLM_ENG_0001](../engineering/FLM_ENG_0001__claude_code.md)) を採用しており、AI が lint 違反に直面する場面が日常的に発生する

## 決定

flame では lint の抑制を以下のルールで扱う。

### 局所抑制 (inline disable コメント) は原則使用しない

- ソース内に書く局所抑制コメントは原則禁止する
- lint 指摘を受けたときの第 1 選択は「コードを修正して指摘を解消する」「設計を変えて指摘自体が出ない形にする」のいずれかである
- 第 2 選択として、当該ルールが flame の方針に合わないと判断される場合、ルール側 (lint config ファイル) でグローバル無効化する

### lint config ファイルにはコメントを書かない / 理由は ADR §影響 に書く

- lint config ファイル (`.golangci.yaml` / `.markdownlint-cli2.yaml` / `.shellcheckrc` / `.yamllint` 等の repo root 直下汎用 config と、 各 module 直下に置かれる module 専用 lint config 一切を含む) には **コメントを一切書かない**。 機械可読な設定値のみを置く
- ルールをグローバル無効化する場合、 「なぜ flame ではこのルールが不適切か」 という理由は **当該無効化を要請する ADR の §影響 セクション** に記述する。 lint config 側は当該 ADR と紐付かない単なる設定値として保つ
- AI エージェントが context window に lint config を取り込む際の token 消費・通信コスト・推論時間を縮小する観点と整合する ([FLM_APP_0010](../application/FLM_APP_0010__code_comment.md) §背景 と同じ動機)
- 設計選択を ADR §影響 に集約することで、 「なぜそうなっているか」 を ADR から辿る単一情報源運用に揃う ([FLM_GEN_0001](FLM_GEN_0001__adr.md))
- lint config 側のコメント混入抑止は静的検査では完結させず、 AI レビュー hook (FLM_APP_0010 §lint で参照される redundant-comment-remover の scope に lint config を含める形) と人レビューで担保する

### 局所抑制が真に避けられない場合のみ、理由を併記して例外的に許す

- ツール由来の真の false positive で、グローバル無効化すると他の正当な検出まで失う場合に限り、局所抑制を例外的に許容する
- 局所抑制コメントの直前または直近に、日本語で理由を書く
- 理由には「なぜ false positive と判断したか」「なぜグローバル無効化や設計変更で回避できないか」を書く

### 設計変更で抑制不要にできる場合はそちらを優先する

- グローバル無効化 / 局所抑制 / 設計変更の 3 案を並べたとき、設計変更で抑制不要にできるならそれを優先する

## 影響

- lint 違反に直面したときに取れる経路が「コード修正」「設計変更」「config 側のグローバル無効化」「理由付き局所抑制 (例外)」に固定される
- ルール無効化の対象が config ファイル側に集約されるため、リポジトリ内で無効になっているルール集合が config ファイル 1 箇所から見渡せる
- グローバル無効化の理由は ADR §影響 に書かれるため、 後続レビュアー (人間 / AI) は ADR から「なぜそうなっているか」 を一次情報源として辿れる。 lint config 側はコメントを持たないため設定値の機械可読性が保たれる
- 局所抑制 (`//nolint:rule // 理由`) は例外として許容され、 当該 1 行直近に日本語で理由を書くことで判断の合理性を残す
- ルールごとのグローバル無効化を選んだ場合、リポジトリ全体で当該ルールの検出力が失われる
- 抑制を回避するために設計変更が選ばれる頻度が増え、設計判断の選択肢に「lint 指摘を出さない書き方を選ぶ」が常時加わる
- lint config 側のコメント混入を AI レビュー hook で機械的に剥がすため、 既存の redundant-comment-remover (FLM_APP_0010 §lint) の scope に lint config を含める運用に揃う

## 評価

代替案として以下を検討した。

- **局所抑制を自由に許容する**: lint の検出力は「抑制が稀である」前提で初めて意味を持つ。抑制が日常になると、検出された違反のうち合理的なもの (修正すべき) と不合理なもの (抑制で済ませる) の境界が曖昧化し、レビュー時の判断負荷が抑制 1 件ごとに発生する。flame の静的チェック優先方針 ([FLM_GEN_0004](FLM_GEN_0004__static_check.md)) と整合しないため不採用。
- **一切の抑制を禁止する**: ツール由来の真の false positive (検証済みのバグ、対応待ちの ML 系ルールの誤検出等) を回避できず、開発が止まる場面が現実に存在する。例外として「理由付きの局所抑制」「config 側でのグローバル無効化」を許容する形を採用した。
- **理由記述を任意とする**: 抑制 / 無効化が増えたときに、後続レビュアーが「なぜ抑制されているか」を読み取れず、合理性チェックが効かなくなる。理由の必須化を採用した。
- **理由を英語で書くことも許容する**: ドキュメント・ソースコメントの自然言語規約 ([FLM_APP_0001](../application/FLM_APP_0001__document.md)) に揃えるため、抑制理由も日本語に統一する。
- **抑制 vs グローバル無効化の優先順位を付けない**: 局所抑制が散在すると影響範囲が読み取りにくくなる。「ルール自体が不適合 → config 側で無効化」「真の false positive → 局所抑制」と分けることで、抑制の意図 (リポジトリ方針なのか、特定箇所の例外なのか) を表現する側を採用した。
- **グローバル無効化の理由を lint config 側コメントに書く**: 設定値と理由が物理的に近い利点があるが、 (1) AI エージェントが lint config を context window に取り込む際にコメントが token を消費する ([FLM_APP_0010](../application/FLM_APP_0010__code_comment.md) §背景 と同じ動機)、 (2) 同じ無効化を要請する ADR §影響 と二重管理になり ADR 側更新時に config 側コメントが取り残される drift を生む、 (3) lint config が機械可読な設定とドキュメント混在ファイルになり責務が曖昧化する、 という不利益がある。 理由は ADR §影響 に集約し lint config 側はコメント無しに統一する方を採用した。
- **lint config を ADR と独立した自由な notes 領域として扱う**: 設定値の意図メモを config 側に書ける柔軟性が得られるが、 ADR を一次情報源とする運用 ([FLM_GEN_0001](FLM_GEN_0001__adr.md)) と競合し AI / 人ともに「どこを見れば本当の理由が分かるか」 が分散する。 ADR §影響 に集約する方を採用した。
