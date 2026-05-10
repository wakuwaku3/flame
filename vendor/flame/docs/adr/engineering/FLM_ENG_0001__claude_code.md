# AI 開発 harness として Claude Code を採用する

## 背景

- flame は AI エージェントと協働した開発を前提として設計する ([FLM_GEN_0002](../general/FLM_GEN_0002__flame.md))
- flame の品質保証は 3 層 FB ループで構成し、その 1 層目を AI ターン内 hook (ターン終端 hook / ツール呼び出し前 hook) で実装する ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md))
- ルール検査は静的検査を最優先とし、静的化が困難な部分のみ AI ヒューリスティック検査で補完する ([FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md))
- 各コンテンツ種別は作成 skill / lint / build / test / ADR ルール検査 skill の 5 項目で品質保証を組み立てる ([FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md))
- Claude Code は Anthropic 公式の CLI 形式 AI 開発 harness であり、AI のターン内で起動する hook (ターン終端で発火する Stop hook、AI のツール呼び出し前に発火する PreToolUse hook 等)、再利用可能な指示集合 (skill / slash command / agent)、ファイルパスベースで自動的にコンテキストへ注入されるルール (rule) などの拡張機構を持つ
- Claude Code の skill は description (AI に常時読まれる) と本文 (description の trigger 判定でマッチした時のみ展開される) の二段構造で、関連スクリプト・テンプレート・参照ドキュメントを同ディレクトリに同梱できる
- Claude Code の skill は frontmatter の `name` フィールドで識別され、その値は仕様上 lowercase 英字・数字・ハイフンのみで構成される
- Claude Code は project root および各 subdirectory に置かれた `CLAUDE.md` を session 内に自動注入する (root の `CLAUDE.md` は session 起動時に常時、subdirectory の `CLAUDE.md` は当該 directory 配下のファイルが context に入った時)
- Claude Code の他の context 注入機構 (rule の `paths:`、skill の description trigger) は条件付きで context に載る一方、`CLAUDE.md` は条件付与の余地がなく対象 directory に応じて無条件に注入される
- Root の `CLAUDE.md` は無条件に毎セッション注入されるため、 ファイル長 (記述量) が定常的な token 消費・通信コスト・推論時間に直接反映される。 同じ記述を ADR 側に置けば該当 ADR が参照された時のみ token に乗るが、 Root `CLAUDE.md` に置いた記述は session 起動時から終了まで常時 token に乗る

## 決定

flame は AI 開発の harness として **Claude Code** を採用する。Claude Code が提供する各機構の使い方は以下に定める。

### AI ターン内 hook の構成

[FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md) で定める AI ターン内 hook は Claude Code の Stop hook と PreToolUse hook で実装する。検査の重さに応じて起動契機を以下のように分担する。

#### Stop hook (ターン終端、静的検査)

ターン終端 hook では当該ターンで変更されたコンテンツに対する静的検査 ([FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md)) を起動する。

- AI が応答を返そうとした時点で、当該ターンで変更されたコンテンツに対する静的検査を起動する
- 検査が違反を検出した場合は Stop をブロックし、違反内容を AI に返して同一ターン内で修正サイクルを回す
- 当該ターンの変更が検査対象外の場合は、hook は早期 return して空振りさせる

#### PreToolUse hook (`git push` 直前、AI レビュー)

ツール呼び出し前 hook では、AI が `git push` を実行しようとした時点で AI ヒューリスティック検査 ([FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md)) を AI レビューとして起動する。違反検出時はツール呼び出しをブロックし、違反内容を AI に返却 → AI が同一ターン内で修正 (新規 commit を積み上げる形での fix を基本とし、既存 commit の amend / rebase は要求しない)、というループで fix する。

検査対象は「現在の HEAD と当該ブランチの upstream remote-tracking ref との差分ファイル集合」とする。push のコマンド形態に応じて以下のルールを適用する。

- **通常の `git push` 形態** (bare `git push`、`-u` / `--set-upstream`、 remote / refspec 指定の組み合わせ): 検査対象を `git diff <upstream>...HEAD --name-only` で抽出し AI レビューを起動する。upstream は当該ブランチの remote-tracking ref (`@{u}`) を一次経路とし、未設定 (初回 push 等) の場合は `origin/main` を fallback として用いる
- **`--dry-run` を含む形態**: 実 push を伴わないため hook 側で素通りさせる
- **`--delete` / `-d` (remote ブランチ削除) を含む形態**: 検査対象となる差分が概念上存在しないため hook 側で素通りさせる
- **単一 Bash 呼び出し内で他コマンドと chain した形態** (`&&` / `;` / `|` / `||` 等の制御構文): 先行コマンド (典型的には `git commit` / `git pull --rebase` / `git rebase`) が HEAD を変更するため、hook 発火時点 (Bash 実行前) では HEAD が push 直前状態と一致しない。hook で reject し、HEAD を変更する操作と `git push` を別 Bash 呼び出しに分けて再実行する旨を AI に指示する

これらの reject / 素通り条件は AI 向けの規範ルールとしては rule や CLAUDE.md ではなく hook 側で強制する。AI が rule を破る可能性を残さず、対象未確定での AI レビュー誤起動を機構的に防ぐため。

#### AI レビューの構成

- AI レビューは複数の観点で構成する
- 観点ごとに独立した AI agent を割り当てる
- 観点は段階に分けて配置する。段階間は直列、同一段階内は並列で実行する
- 段階間に優先度を設け、優先度の高い段階を後段に配置する
- 同一段階に配置できるのは互いに修正対象が排他的な観点のみとする (修正の競合を避けるため)
- 観点には常時起動するものと、特定種別の変更があった場合のみ起動するもの (条件起動) がある

#### Subagent としての配置と起動

AI レビューの各観点は Claude Code の subagent として独立に定義する。PreToolUse hook script 自体は subagent を直接起動せず、block の reason に Task tool 経由の起動順を書き、親セッションが Task tool で各 subagent を順に起動する責務分担とする。

各 subagent 定義 (frontmatter + system prompt) には以下を含める。

- 担当する観点 (役割 / 観点リスト)
- 担当外 (他観点 subagent の責務との境界の明示)
- 変更ファイルの特定方法 (PreToolUse hook が block の reason 内で push 対象差分のファイルリストを直接渡すため、subagent は受け取ったリストをそのまま検査対象として使用する)
- 出力形式 (違反は箇条書き、違反ゼロは `No findings.` のみを返す)

条件起動の観点については、PreToolUse hook script が当該 push の対象ファイル種別を判定し、起動対象の subagent リストを段階構造ごと block の reason に書き込む。親セッションは同一段階内に列挙された subagent を Task tool で並列起動し、全ての違反 fix 後に次の段階へ進む。

ADR の追加・更新や観点の追加・削除に伴い、該当する subagent の system prompt も同期更新する。

##### 段階 2 reviewer の検査スコープ

段階 1 reviewer に渡される検査対象は PreToolUse hook の `(path, hash)` state による絞り込みで「前回レビュー以降に内容が変わったファイル」に限定される。 同一 push attempt 内の fix 効率化のための機構だが、 ADR の §決定 で示された実装に対する規約 (配置・命名・階層・モジュール境界・公開 surface 等) への違反は、 実装ファイルが変わらない限り当該機構によって永続的に検査対象から外れる。 初回レビューで段階 1 / 段階 2 のいずれもが当該違反を見逃した場合、 以降同一 push attempt 内では再検出機会を失う。

これを構造的に防ぐため、 段階 2 reviewer (現行 `adr-reviewer`) には以下のスコープを与える。

- (a) **変更経路**: 段階 1 と同じく「前回レビュー以降に内容が変わったファイル」に対し、 当該変更それ自体が ADR §決定 を破ったかを精査する
- (b) **ADR 整合性経路**: PreToolUse hook が抽出した push 対象差分の **全ファイル** に対し、 ADR §決定 で示された実装に対する規約との整合性を精査する。 hash 差分絞り込みは適用しない

PreToolUse hook script は段階 2 reviewer 起動指示の block reason に両リスト (= 段階 1 検査対象と全 push 対象差分) を含め、 段階 2 reviewer が「前回 review 以降の変更で発生したものではない」 「既存違反である」 等を理由に検出を保留しない旨を明示する。

##### Reviewer registry の同期義務

reviewer subagent の追加・削除・改名・段階配置変更は、本 ADR §影響 の reviewer 列挙 (現行 reviewer・段階配置・起動条件) と同期して更新する。 PreToolUse hook 本体 (flame CLI の `flame ai hook pre-push` 実装) と subagent 定義 (`.claude-plugin/agents/`) のいずれかを変更する PR は、 本 ADR §影響 の registry を併せて更新する責務を負う。 registry 同期を伴わない当該領域の変更は approve しない。

この責務は本 ADR を一次情報源とし、 個別 reviewer 観点の ADR (例: [FLM_APP_0009](../application/FLM_APP_0009__test.md)) は本 ADR を参照する形で発見性を確保する。

### CLAUDE.md (project instruction) の書き分け

Claude Code が context に自動注入する `CLAUDE.md` は、他の機構 (rule / skill / ADR) と役割を分離して以下のように使い分ける。

#### Root の `CLAUDE.md`

`CLAUDE.md` は補助ドキュメントの 1 種であり、補助ドキュメント全般に適用される「ルール記述の単一情報源」規約 ([FLM_GEN_0001](../general/FLM_GEN_0001__adr.md)) に従う。Root `CLAUDE.md` 固有の追加点として以下を満たす。

- Root `CLAUDE.md` は最小限の分量に保ち、 肥大化させない
- 毎ターン AI に必ず守らせたい最重要ルールに限り、 ADR への相対リンクを併記したうえで短文として本文に直書きする ([FLM_GEN_0001](../general/FLM_GEN_0001__adr.md) §ルール記述の単一情報源 で許容される Root `CLAUDE.md` 固有の例外)
- ADR 索引 (どの ADR が存在するか) は本文に持たない。 `docs/adr/` 配下の階層と `.claude/rules/` 配下の rule マッピングが索引の役割を担う
- 編集対象から該当 ADR を辿るための navigation 案内 (どこにルール本文があるか / どこに種別ごとのマッピングがあるか) を本文に置く

#### モジュール内の `<module>/CLAUDE.md`

当該モジュール配下のファイルが context に入った時にのみ注入される性質を活かす。記述内容は補助ドキュメント全般の単一情報源規約 ([FLM_GEN_0001](../general/FLM_GEN_0001__adr.md)) に従い、当該モジュールに固有のドキュメント (例: `<module>/README.md`) や該当 ADR への参照のみで構成する。

### Skill (再利用可能な工程指示) の作成基準と本文構成

Claude Code の skill は以下のルールで作成・運用する。

#### 作成基準

skill は以下のいずれかに該当する場合のみ作成する。

- 工程に複数ステップがあり、いずれかの省略が事故 (回帰・不整合・実行時エラー等) を直接生む場合
- 工程内で同梱する script / template / 参照ドキュメントを伴い、skill ディレクトリとしてまとまった単位で参照させたい場合
- ユーザが明示的にトリガーする工程がある場合

ADR + 既存ファイルを読めば達成できる単発・宣言的な作業に対しては skill を作成しない。

#### 本文構成

skill 本文は補助ドキュメント全般に適用される「ルール記述の単一情報源」規約 ([FLM_GEN_0001](../general/FLM_GEN_0001__adr.md)) に従う。skill 固有の追加点として以下を満たす。

- 該当する ADR が存在する場合、冒頭で ADR ID を相対リンクで参照する
- description は AI が trigger 判断できる粒度で書く (どのような工程の開始時に起動すべきかが一読で分かること)

#### 命名

- 対応するコンテンツ種別 / rule / ADR がある場合、skill 名にそれらと同じ語幹を用いる

#### ADR との同期

ADR の追加・更新時には、対応する skill (存在する場合) を同期更新する。

### Plugin (Claude Code 拡張の配布) の構成

flame の Claude Code 拡張のうち plugin の正式配布 component (agents / skills / hooks) は repo root の `.claude-plugin/` を SoT として配布する。 配布チャネルの全体構成は [FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §チャネル A に従う。

- 配置: `.claude-plugin/plugin.json` (manifest) + `agents/` + `skills/` + `hooks/hooks.json`
- plugin.json の `name` は repo 識別子と揃える (flame の場合は `flame`)、 `version` は flame ツール単一 version と同期
- rules は plugin の正式配布 component に含まれないため plugin 経路では配布しない (project-local 機構として `.claude/rules/` で運用、 配布は [FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §チャネル C)
- flame 自身も dogfooding として plugin を外部参照経路で読み込む (Claude Code セッションでの `/plugin marketplace add wakuwaku3/flame` → `/plugin install flame@flame`、 開発時の ad-hoc ロードは `--plugin-dir .claude-plugin/`)
- 利用側拡張は副ファイル overlay 不可。 利用側で agent / skill / hook を追加する場合は project-local `.claude/agents/` / `.claude/skills/` / `.claude/settings.json` に **異なる名前** で追加する (Claude Code 側の namespace 機構により plugin 側と共存)
- hook command 内で plugin 内ファイルを参照する場合は Claude Code の `${CLAUDE_PLUGIN_ROOT}` を用いる。 cwd は session working directory 側のため、 hook 実行コンテキストは利用側 repo に対して効く

## 影響

- 現時点で運用している観点は 5 つで、段階 1 (並列実行) に「一般的な技術プラクティス観点」(常時起動) と「rule-ADR 整合性観点」(当該 push 対象差分に ADR の追加・更新が含まれる場合のみ起動) と「test 充足度観点」(当該 push 対象差分に Go 実装ファイル / Go test ファイルが含まれる場合のみ起動) と「冗長コメント削除」(当該 push 対象差分に `*.go` / `*.sh` が含まれる場合のみ起動、 違反指摘ではなく Edit / Write で削除を直接実行する specialized reviewer) を、段階 2 (単独実行) に「ADR 観点」(常時起動) を配置している。各観点に対応する subagent はそれぞれ `general-practices-reviewer` / `rule-adr-sync-reviewer` / `test-coverage-reviewer` / `redundant-comment-remover` / `adr-reviewer`。rule-ADR 整合性観点は [FLM_GEN_0001](../general/FLM_GEN_0001__adr.md) で規約化されている補助ドキュメント (rule / skill / `CLAUDE.md` / `README.md`) と ADR の同期 (ADR リンク・`paths:` 等の metadata の整合) を検査する。test 充足度観点は [FLM_APP_0009](../application/FLM_APP_0009__test.md) で定めた policy への準拠とテストケース抽出の十分性をヒューリスティックに評価する。冗長コメント削除観点は [FLM_APP_0010](../application/FLM_APP_0010__code_comment.md) §書かない対象 に該当するコメントを `*.go` / `*.sh` から削除する (返却される削除レポートを親セッションが受理し、 該当判定のやり直しは行わない。 他 reviewer の指摘 fix と修正対象が排他的なため段階 1 内並列に置く)。ADR 観点 (段階 2 / `adr-reviewer`) は §段階 2 reviewer の検査スコープ で定める (a) 変更経路 + (b) ADR 整合性経路 の二経路で検査する
- AI ターン内で検査が完結し、誤った成果物がリモートへ公開される前に AI 自身が修正サイクルを回せる
- Claude Code の harness 仕様 (Stop hook / PreToolUse hook の event 名、設定ファイルのパス・スキーマ、hook script の入出力プロトコル等) に flame の運用が依存する。Claude Code の仕様変更時には設定の追従が必要となる
- 静的検査は Stop hook で毎ターン起動するが軽量なため累積コストが小さく、AI レビューは PreToolUse hook で `git push` 直前にのみ起動するため push を伴わないターンではコストが乗らない (起動頻度を検査の重さに応じて分離する [FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md) の方針に従う)
- 別の AI 開発 harness (Cursor、Cline 等) を主軸とする開発者は、開発時の harness を Claude Code に切り替えるか、Stop hook / PreToolUse hook 等と同等の機構を別 harness で再実装する必要がある
- 依存側プロジェクトにも Claude Code 採用が伝播する (本 ADR は ENG カテゴリのため)
- AI レビューの段階数を増やすほど PreToolUse hook 起動時のレイテンシが線形に増える (段階間は直列のため)。同一段階内に観点を追加した場合は並列実行のため最も遅い観点のレイテンシのみが効く
- 同一段階内の観点同士は修正対象が排他的であることを設計時に保証する必要がある (排他性が崩れると修正の競合が再発する)
- 条件起動により、当該 push 対象差分のファイル種別と無関係な観点 (例: ADR 変更を含まない push に rule-ADR 整合性観点) は起動されず、定常的なレイテンシ・トークンコストを抑えられる
- 条件起動の判定 (どの種別のファイルが push 対象差分か) は PreToolUse hook script が担うため、起動条件のロジックが script 側に集中する
- PreToolUse hook は `(path, hash)` の state ファイルを保持し、 reviewer に渡す検査対象を「前回レビュー以降に内容が変わったファイル」のみに絞る。 同一 push attempt 内で AI が一部ファイルを fix commit として積み上げた場合、 fix が乗っていないファイルは reviewer の attention 対象から外れるため、 reviewer が pass ごとに視点を変えて新規指摘を出すことによる修正ループを抑止できる。 push 対象差分の全ファイルが state の hash と一致する場合は素通り (= AI が指摘を消化したと判断)。 upstream remote-tracking ref が動く (= push 成功または `git fetch` で前進する) と state は失効する
- PreToolUse hook の reject / 素通り条件 (chain 結合・`--dry-run` / `--delete`) も hook script に集中するため、AI 向けの規範ルールを rule や CLAUDE.md に分散させずに済む。AI が rule を破った場合でも hook が機構的にブロックする
- 違反 fix は「新規 commit を積み上げる」運用が基本となるため、PR の commit 履歴に "fix: review feedback" 相当の修正 commit が混在する。履歴の意図を圧縮したい場合は merge 時の squash で吸収する
- 観点ごとに独立 agent のため、観点間でレビュー文脈は直接共有されない (前段の判断理由は後段の agent には渡らず、前段が誘導した修正後の成果物のみが後段の入力となる)
- 観点を追加・削除するたびに、段階配置と起動条件の見直し、および対応する subagent 定義 (`.claude/agents/`) の追加・削除が必要となる
- PreToolUse hook が subagent を直接起動せず親セッション経由で起動する構成のため、親セッションのコンテキスト (これまでのユーザ指示・編集履歴) を Task tool プロンプト経由で subagent に渡せる
- subagent 起動と fix の役割分担が固定される (subagent は違反返却のみ、fix は親セッションが担う)
- 段階間で判断が衝突した場合、後段の観点が示した修正が最終形として残る (前段が要求した修正が後段の修正で覆りうる)
- 横断的指示は `CLAUDE.md`、ファイル種別ルールは rule、工程手順は skill、決定は ADR、と書く場所が機械的に決まる
- root `CLAUDE.md` は無条件に毎 session 注入されるため、ここに書く内容を最小化することで定常 context コストを抑えられる
- Root `CLAUDE.md` は ADR への参照リンクと最重要ルールの短文直書きで構成されるため、 ADR 改訂時の `CLAUDE.md` 側の追従コストは、 リンクの維持と (短文直書きしている ADR が改訂された場合の) 短文側の同期更新に限定される
- モジュール内 `CLAUDE.md` は当該モジュール編集時のみ注入されるため、root `CLAUDE.md` と内容の性質 (具体実装 vs 横断指示) を区別する判断が新規作成時に必要となる
- skill 本文は ADR を引用しないため、ADR 改訂時に skill 側の追従コストはリンクと procedural 部分のみに限定される
- skill 作成の閾値が明示されるため、過剰な skill 増殖 (宣言的ルールまで skill 化) と過少な skill 整備 (procedural な事故を ADR だけで防ごうとする) の両端を避ける根拠ができる
- skill の description は常時 system prompt に載るため、skill 数の増加に応じて定常的な context コストが増える (本文は trigger 時のみ展開のため定常コストには乗らない)
- skill 名と rule 名・ADR タイトルが揃うことで、3 者の対応関係が機械的に追える
- skill が ADR 内容を複製しないため、skill 本文だけ読んで作業すると ADR の前提を取りこぼす可能性が残る (skill 冒頭の ADR リンクを必ず辿る運用が前提となる)
- plugin 配布対象 (agents / skills / hooks) の SoT が repo root の `.claude-plugin/` に集約され、 vendor (`vendor/flame/.claude/`) からは agents / skills / hooks 関連が削除される
- plugin の利用側拡張は副ファイル overlay 不可となり、 project-local `.claude/` への異名同居 (Claude Code namespace 機構) で対応する。 vendor チャネル ([FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §チャネル C) で許容される overlay 経路とは利用側拡張のモデルが異なる
- plugin manifest version は flame ツール単一 version と同期するため、 flame 側 version bump 時に plugin.json の version も同時に書き換える (CLI 側 release script で自動化)
- rules は plugin 配布対象外のため `.claude/rules/` は引き続き vendor チャネルで配布される。 rules を plugin 配布化する余地は Claude Code plugin 仕様変更時に再評価する

## 評価

代替案として以下を検討した。

- **harness を採用せず、素の AI API + 自作スクリプトで運用する**: AI ターン内 hook、skill 相当の指示再利用機構、rule 相当のコンテキスト注入機構をすべて自作する必要があり、[FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md) の AI ターン内 hook の実装コストが大きく増える。既製 harness の仕様に乗る方が初期コストが低いため不採用。
- **別の AI 開発 harness (Cursor / Cline 等) を採用する**: AI ターン内 hook 相当・skill 相当・rule 相当の機構を持つ harness は複数存在する。(1) 機構の組み合わせと API の安定性、(2) Anthropic 公式 Claude モデルとの統合、(3) CLI 形式により CI 等のヘッドレス環境に組み込みやすいこと、を理由に Claude Code を選択した。
- **複数 harness を同時にサポートする**: 各 harness 固有の設定・skill 形式・hook プロトコルを個別に維持する必要があり、flame 側のメンテナンス対象が増える。harness を 1 つに絞ることで構造を単純化する方針を採用した。

AI レビューの構成について、以下の代替案を検討した。

- **すべての観点を並列に実行する**: 修正対象が重複する観点同士を並列化すると、複数観点が同時に違反を返した場合に AI の修正が観点間で競合し、修正結果が非決定的になる。修正対象が重複しうる観点は段階を分けて直列化し、修正対象が排他的な観点のみ同一段階内で並列化する設計とした。
- **すべての観点を直列に実行する**: 修正対象が排他的な観点 (例: 一般的な技術プラクティス観点と rule-ADR 整合性観点) も直列化すると、本来並列で済む処理に不要なレイテンシが乗る。並列可能な観点は並列化することで PreToolUse hook 全体のレイテンシを抑える。
- **観点を統合した単一 agent でレビューする**: 観点間の優先度制御が agent 内部のプロンプトに閉じ、観点単位の粒度調整 (どの観点をどのタイミングで実行するか、特定の観点を一時的に外すか等) が困難になる。観点ごとに独立 agent とすることで、観点単位の精度と優先度を明示的に制御できる。
- **優先度の高い観点を前段に配置する**: 修正サイクルでは AI が最新のフィードバックに基づいて成果物を再構成するため、前段の判断は後段の判断で覆る可能性がある。優先度の高い観点を最終結果に確実に反映するため、後段に配置する形を採用した。
- **rule-ADR 整合性観点を ADR 観点に統合する (条件起動を導入しない)**: ADR が変更されない commit でも rule との同期チェックを毎回実行することになり、定常的なレイテンシとトークンコストが増す。当該 commit に ADR の変更が含まれる場合のみ起動する条件起動方式を採用した。
- **rule-ADR 整合性チェックを静的検査で実装する**: ADR と rule の対応 (paths の網羅性、ADR リンク切れ等) は静的化可能な部分もあるが、「既存の rule に追記すべきか新規 rule を作るべきか」「補助ドキュメントが要約・チェックリスト等の形で ADR 内容を再記述していないか」といった意図解釈を要する判断が含まれる。当面は AI 検査で扱い、静的化可能な部分は [FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md) の方針に沿って漸進的に分離していく。
- **PreToolUse hook script から subagent を直接起動する (`claude -p` でサブプロセス起動)**: 親セッションを介さない分構造は単純になるが、(1) 親セッションのコンテキスト (ユーザ指示・編集履歴) を subagent に渡せず文脈不足になる、(2) 別プロセスのため認証・モデル選択・トークン会計が二重化する、(3) 違反 fix を hook script 側で扱う必要が生じ、 fix 主体が親セッションから script に分散する。Task tool 経由で親セッションが subagent を起動する形を採用することで、文脈共有と fix 主体の単一化を両立する。
- **ADR §決定 と実装の整合性検査用に新規 reviewer を追加する**: ADR 整合性検査を独立 reviewer として段階 1 / 2 のいずれかに追加すると、(1) 既存 4 reviewer との責務境界の再定義が必要、(2) 段階 1 に追加すると修正対象排他性 (同一段階内の reviewer は修正対象が排他的) を満たせず、 段階 2 に追加すると adr-reviewer との責務重複が大きい。 段階 2 reviewer (`adr-reviewer`) の検査スコープを (a) 変更経路 + (b) ADR 整合性経路 の二経路に拡張する形を採用し、 reviewer 数を増やさず構造的見逃しを塞いだ。
- **段階 1 reviewer の hash 差分絞り込みを廃止する**: 全 reviewer が常に push 対象差分の全ファイルを検査する構成にすれば、 ADR §決定 違反の検出機会は永続的に保たれる。 ただし fix 効率化のための機構が消え、 同一 push attempt 内で AI が一部ファイルを fix commit として積み上げた場合に、 reviewer が pass ごとに視点を変えて新規指摘を出す修正ループ (= attention 対象から外れない既存ファイルへの再指摘) が再発する。 段階 1 では絞り込みを維持し、 段階 2 のみ全ファイル検査とする折衷案を採用した。

`CLAUDE.md` の書き分けについて、以下の代替案を検討した。

- **すべての横断指示・決定を `CLAUDE.md` に集約する**: rule / skill / ADR が空洞化し、定常 context が肥大化する。条件付き注入 (rule の paths、skill の trigger) と無条件注入 (`CLAUDE.md`) の使い分けによる context loading 最適化が崩れるため不採用。
- **`CLAUDE.md` を使わず rule で代替する**: rule は paths 紐付きのため、ファイル種別に依存しない協働モデル等を全 session で常時注入する手段がなくなる。`CLAUDE.md` は paths 条件のない無条件注入機構として保持する。
- **`CLAUDE.md` に ADR 決定本文を直書きする (ADR と重複させる)**: ADR 改訂時に `CLAUDE.md` 側の追従が漏れて長期的に乖離する。最重要ルールの短文直書きに限定し ADR 改訂時の追従更新を義務付けたうえで、 決定本文は ADR を一次情報として保つ方を採用した ([FLM_GEN_0001](../general/FLM_GEN_0001__adr.md) §ルール記述の単一情報源)。
- **Root `CLAUDE.md` の肥大化を許容する**: Root `CLAUDE.md` は無条件に毎セッション注入される機構のため、 記述量はそのまま定常的な token 消費・通信コスト・推論時間として全セッションに乗る。 同じ記述を ADR 側に置けば該当 ADR が参照された時のみ token に乗るのに対し、 Root `CLAUDE.md` に置いた記述は常時 token に乗り続ける。 記述対象を最重要ルールの短文直書きと navigation 案内に絞り、 Root `CLAUDE.md` 全体の分量を最小限に保つことで定常 token コスト増を抑える方を採用した。

skill の作成基準と本文構成について、以下の代替案を検討した。

- **skill の作成基準を設けず必要に応じて自由に作る**: 基準が曖昧だと、宣言的ルールまで skill 化してメンテ対象を増やすか、procedural な事故が顕在化するまで skill 化しないか、どちらかに振れる。基準を明示することで両端を防ぐ。
- **skill の作成基準を「procedural workflow であること」のみに絞る**: 同梱物 (script / template) が必要なケースや、ユーザ明示トリガーが必要なケースを skill 化する根拠を失う。3 つの基準を OR で並べる形を採用した。
- **skill 本文に ADR の内容を複製し skill 単体で完結させる**: ADR 改訂時に skill 側の追従が漏れて長期的に乖離する。複製せずリンクのみとし、ADR を一次情報として保つ方を採用した。
- **skill / rule / ADR を一本化する**: 3 機構は context loading の方式 (progressive disclosure / paths-based injection / on-demand reference) と起動契機が異なる。一本化するとそれぞれの最適化が崩れるため、役割分担した上で併用する方を採用した。
- **skill の作成基準と本文構成を本 ADR ではなく独立 ADR (ENG カテゴリの別 ADR) として切り出す**: skill は Claude Code が提供する 1 機構であり、Stop hook / PreToolUse hook / subagent / `CLAUDE.md` と並列の概念。これらと同じ ADR にまとめる方が「Claude Code の各機構の使い方」を 1 ADR で参照でき、機構間の役割分担 (rule / skill / `CLAUDE.md` / ADR) を 1 ADR 内で対比できる利点が大きい。本 ADR に統合する形を採用した。

plugin 配布について、以下の代替案を検討した。

- **plugin を採用せず `.claude/` 全体を vendor で配布する**: SoT と install 先の二重管理が agents / skills / hooks にも適用され vendor 規模が大きくなる。 Claude Code 標準の plugin 機構に乗せられる component (agents / skills / hooks) は plugin 経路で配布し、 vendor は plugin 配布対象外の component (rules) と plugin で表現できない設定 (`CLAUDE.md` 等) に絞る方を採用した
- **rules も plugin で配布する (`.claude-plugin/rules/`)**: Claude Code plugin の正式 component に rules は含まれず、 project-local 機構として運用される。 plugin 経路で配布しても利用側 Claude Code セッションで paths frontmatter ベースの context 注入が機能しない。 rules は vendor 残置 ([FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §チャネル C) とし、 Claude Code 仕様変更時に再評価する余地を残した
- **plugin manifest version を flame ツール本体と別 semver で採番する**: plugin 単独の breaking change を独立に管理できる利点があるが、 利用側は plugin version + flame ツール version の互換マトリクスを管理する必要が生じる。 flame ツール (CLI + harness + plugin) を 1 つの version で運用する [FLM_FEA_0003](../feature/FLM_FEA_0003__harness.md) §決定 と整合させ、 plugin.json の `version` を flame 単一 version と同期する方を採用した

過去に採用していた決定として以下の経緯がある。

- 当初は本 ADR の skill 本文構成と `CLAUDE.md` 書き分けセクションに、補助ドキュメント全般に共通する「ADR 内容を複製しない」「ADR への参照と procedural / 縮約版要約に限る」というルールを skill / `CLAUDE.md` 個別に書いていた。 同種のルールは rule や README にも適用すべき横断的な policy であるため、これを上位 ADR ([FLM_GEN_0001](../general/FLM_GEN_0001__adr.md)) の「ルール記述の単一情報源」セクションに集約し、本 ADR の skill / `CLAUDE.md` セクションからは当該規約への参照と skill / `CLAUDE.md` 固有の追加点 (skill 名命名、description の trigger 判断粒度、root と module 内の `CLAUDE.md` 書き分け等) のみを残す形に整理した。
- 当初は root `CLAUDE.md` を「rule / skill / ADR で表現できない横断指示の置き場」と定義し、AI の判断流儀・協働モデル等を本文に直書きする運用としていた。 補助ドキュメント本文に ADR 内容や独自ルールを置かない方針 ([FLM_GEN_0001](../general/FLM_GEN_0001__adr.md) §ルール記述の単一情報源) を全面適用することに伴い、これらの横断指示は ADR 側 (例: 協働モデルは [FLM_GEN_0002](../general/FLM_GEN_0002__flame.md) §AI との協働モデル) へ集約し、root `CLAUDE.md` は ADR への参照リンクの列挙のみで構成する形に整理した。同様にモジュール内 `CLAUDE.md` も「ADR 化に値しない具体実装ルールの直書き場」から「モジュール固有ドキュメントや該当 ADR への参照置き場」へ位置付けを改めた。 モジュール固有の事情で ADR 化が困難な実装ルールが必要になった場合は、当該モジュール内の README 等の文書 (補助ドキュメント全般の単一情報源規約に従ったうえで) に置く運用とする。
- 当初は AI レビューを直列実行のみで構成していた (前段: `general-practices-reviewer` → 後段: `adr-reviewer` の 2 段)。観点間の修正競合を避ける目的で並列を排除していたが、`.claude/rules/` の整備に伴い rule-ADR 整合性観点を追加する際、修正対象が一般的な技術プラクティス観点と排他的であることが明確だったため、段階内並列を許容する形に拡張した (段階間は引き続き直列)。同時に、変更内容と無関係な観点を毎回起動する無駄を避けるため、条件起動の概念を導入した。
- 当初は AI レビューも静的検査と同じく Stop hook で毎ターン起動する構成だった。Stop hook はターン終端で必ず発火するため、軽量編集を繰り返すターン (ドキュメント修正の小分け、設定値の細かい調整、応答での会話的なやり取り等) でも AI レビューが起動し、トークンコストとレイテンシが累積していた。AI レビューは「変更を commit としてリポジトリに固定する直前」に実行することが本質であり、`git commit` がそのタイミングとして最も自然なため、PreToolUse hook で `git commit` 直前に AI レビューを起動する構成に変更した。Stop hook は引き続き静的検査の起動に使用する ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md) で「AI ターン内 hook」の起動契機を 2 種類 (ターン終端 hook + ツール呼び出し前 hook) に拡張した方針に対応)。あわせて、PreToolUse hook の対象ファイル算出を `git commit --dry-run --porcelain` で git 自身に行わせる形を採用した。bare commit / `-a` / pathspec のすべての形態で git 側に解決させるため hook では commit 形態ごとの分岐ロジックを持たない。`--amend` のみ「amend 後 commit の全ファイル」ではなく「amend で新たに加わる差分」をレビュー対象としたいため、hook 側で `--amend` を事前除去してから dry-run を呼ぶ (除去後の args は通常 commit と等価のため、git のセマンティクスに沿って差分のみが得られる)。対象が静的に決定できない形態 (`-p` / `--patch`) と、staging が hook 発火時点で未確定となる形態 (`&&` / `;` 等で他コマンドと chain した呼び出し) は hook 側で reject し、AI に等価な代替形態 (`git add` でステージング → 別 Bash 呼び出しで `git commit`) での再実行を指示する。
- 当初 AI レビューは段階 1 に `general-practices-reviewer` / `rule-adr-sync-reviewer`、 段階 2 に `adr-reviewer` の 3 reviewer 構成だった。 [FLM_APP_0009](../application/FLM_APP_0009__test.md) の test ルール整備に伴い、 test 充足度観点を担う `test-coverage-reviewer` を段階 1 に追加して 4 reviewer 構成に拡張した。 この拡張時に `scripts/pre-push-review.sh` と `.claude/agents/test-coverage-reviewer.md` は更新されたが、 本 ADR §影響 の reviewer 列挙 (3 reviewer のまま) との同期更新が漏れ、 ADR が一次情報源としての整合を失う状態が一時的に発生した。 同期漏れを再発させないため、 §AI レビューの構成 §Reviewer registry の同期義務 を新設し、 reviewer の追加・削除・改名・段階配置変更が §影響 の registry 同期更新を伴わない場合は approve しない policy を明文化した。 本 ADR を reviewer registry の一次情報源とし、 個別 reviewer 観点の ADR (例: [FLM_APP_0009](../application/FLM_APP_0009__test.md)) は本 ADR を参照する。
- 当初は全 reviewer が PreToolUse hook の `(path, hash)` state による絞り込み済みリスト (= 前回レビュー以降に内容が変わったファイル) のみを検査対象としていた。 同一 push attempt 内の fix 効率化のための機構だが、 ADR §決定 で示された実装に対する規約 (配置・命名・階層・モジュール境界・公開 surface 等) への違反は、 実装ファイルが変わらない限り絞り込み機構によって永続的に検査対象から外れる構造を持っていた。 初回レビューで全 reviewer が当該違反を見逃した場合 (= 4 reviewer の責務分担に「ADR §決定 と実装の整合性」を主担当する位置が無く、 段階 2 の `adr-reviewer` が「変更ファイル限定」の解釈で既存違反を「scope 外」として却下する判断を許容していたため、 違反が観測された)、 以降同一 push attempt 内では再検出機会を失う。 これを構造的に防ぐため、 §AI レビューの構成 §段階 2 reviewer の検査スコープ を新設し、 段階 2 reviewer に (a) 変更経路 (絞り込み済み) と (b) ADR 整合性経路 (push 対象差分の全ファイル) の二経路を与え、 (b) では「既存違反である」 等を理由に検出を保留しない policy を明示した。 hash 絞り込みによる効率化は段階 1 のみに留め、 段階 2 では絞り込みを行わない構成とした。
- 上記改訂後は AI レビューを `git commit` 直前 (PreToolUse hook の matcher = `git commit`) で起動していた。 commit 区切りでのレビューは (1) WIP の小分け commit を作るたびに重いレビューが走りトークン / レイテンシが累積する、(2) 違反指摘での fix が「既存 commit の amend / rebase」になり AI の操作難易度・事故リスク (履歴改変・conflict 解決失敗) が高い、(3) リモート公開前の境界として `git push` のほうが「他者の目に触れる前」という AI レビューの本質的タイミングに近い、 という観察が累積したため、 PreToolUse hook の matcher を `git push` 直前へ移し、 fix は新規 commit を積み上げる運用 (既存 commit の amend / rebase を要求しない) に変更した ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md) 側の「commit という区切り単位 → push という区切り単位」改訂に対応)。あわせて、 検査対象ファイル算出を `git commit --dry-run --porcelain` から `git diff <upstream>...HEAD --name-only` ベース (upstream は `@{u}` を一次経路、未設定時は `origin/main` を fallback) に変更した。 hook の reject / 素通り条件は (chain 結合 / `--dry-run` / `--delete`) の 3 つに整理し、commit 形態に紐付いていた `-p` / `--patch` reject や未 stage 検出は不要となったため削除した。 hook の state 失効条件は HEAD 変化から upstream remote-tracking ref 変化に移し、 push が成功して remote-tracking が前進した時点で state が自然に失効する (新規 commit を積み上げる fix では HEAD は動くが state は保持され、 内容が変わったファイルだけが再レビュー対象になる)。
- 当初は Root `CLAUDE.md` を「ADR への参照リンクの列挙のみ。 列挙以外の本文を持たない」 と定義していた。 (1) ADR 索引が `.claude/rules/` 配下のマッピングと 2 重管理化していたこと、 (2) Claude Code が Root `CLAUDE.md` を無条件に毎セッション注入する機構特性を活かして AI に毎ターン必ず守らせたい最重要ルールを直書きしたいニーズが顕在化したこと、 を踏まえ、 (a) ADR 索引は持たず `docs/adr/` 配下構造と `.claude/rules/` 配下マッピングに一本化、 (b) AI に毎ターン必ず守らせたい最重要ルールに限り短文直書きを許容 (ADR への相対リンク併記と ADR 改訂時の追従更新を義務化)、 (c) 編集対象から該当 ADR を辿る navigation 案内を本文に置く、 という形に改訂した。 補助ドキュメント全般に適用される単一情報源規約 ([FLM_GEN_0001](../general/FLM_GEN_0001__adr.md)) 側にも Root `CLAUDE.md` 限定の例外条項を追加した。
