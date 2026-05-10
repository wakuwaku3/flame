# テストの基本ルール

## 背景

- flame は新規アプリケーション開発の DX を改善する framework である ([FLM_GEN_0002](../general/FLM_GEN_0002__flame.md))
- flame は AI エージェントとの協働開発を前提として設計する ([FLM_GEN_0002](../general/FLM_GEN_0002__flame.md))
- flame では Go を主開発言語に採用し、 test は test を含む package に対して `go test` を実行する ([FLM_APP_0007](FLM_APP_0007__go.md))
- flame の CLI は cobra wrapper (`<lib_module>/clix/`) を介して subcommand ツリーで構成され、 利用者から見える 1 入口は 1 subcommand 起動となる ([FLM_APP_0008](FLM_APP_0008__cli.md))
- flame は静的検査と AI ヒューリスティック検査を組み合わせて品質保証を行い、 静的化が困難な部分のみ AI 検査を導入する ([FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md))
- flame の AI レビューは `git push` 直前 hook (PreToolUse) で起動し、 違反を AI ターン内で fix する ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md))
- flame ではコンテンツ種別ごとに「作成 skill / lint / build / test / ADR ルール検査 skill」の 5 項目を整備する ([FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md))
- 自動テストの実装には複数の時間スケールと粒度が存在する: 関数 1 つを単独で呼ぶレベル、 利用者から見える 1 入口を起動するレベル、 複数入口を組み合わせて利用者シナリオを再現するレベル
- 外部依存 (DB / API / 時計 / ファイルシステム / プロセス境界等) の置換手段には mock (呼び出しの記録 / 期待値の宣言を中心とする差し替え) と fake (本物に近い動作を持つ軽量実装。 emulator・in-memory 実装等を含む) の 2 系統が広く採用されている。 mock は対象コードが「どう呼んだか」 を検証し、 fake は対象コードが「実際にどう振る舞うか」 を検証する
- Go 標準の `testing` package は test を含む package と同居する `*_test.go` を test 単位として扱い、 同 package 内 (white-box) または `<pkg>_test` (black-box) のいずれかで記述できる
- e2e (複数 endpoint を組み合わせた利用者シナリオベース) の test は、 単一 package 内に閉じない構造になり、 配布バイナリ / 起動済みプロセス / 複数 endpoint 横断の状態遷移を扱う点で service-level test と性質が異なる

## 決定

flame では自動テストを「関数単位」 / 「利用者から見える 1 入口単位」 / 「複数入口を組み合わせたシナリオ単位」 の 3 layer に分け、 各 layer に明確な役割を割り当てる。

### test layer

flame の自動テストは以下 3 layer で構成する。

- **service-level test**: 利用者から見える 1 入口 (= endpoint) を 1 単位として、 入力と外部観測可能な出力 (戻り値・標準出力・標準エラー・終了コード・副作用) を検証する black-box テスト。 内部実装の構造には立ち入らない
- **unit test**: 関数 (またはそれに準ずる小さな実装単位) を対象とする white-box / black-box テスト
- **e2e test**: 複数の endpoint を組み合わせた利用者シナリオを再現するテスト

### endpoint の単位

「endpoint」 (= service-level test の 1 単位) は利用者から見える 1 入口を指す。 形態に応じて以下を 1 endpoint とみなす。

- CLI: 1 subcommand 起動 (root command 直起動を含む)
- web server (将来): 1 HTTP route handler の 1 起動
- GitHub Actions ワークフロー: 1 ワークフローファイル (`trg__*.yaml` / `wf__*.yaml`) を 1 endpoint とみなす。 当該 endpoint に対する service-level test の具体規約 (配置先 / 検査軸 / 必須化) は [FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md) §test の必須化と配置 で定める
- その他: 利用者 / 外部システムからの 1 起動経路

内部呼び出し用の helper / library 関数は endpoint ではない。

### service-level test を主軸とする

flame では service-level test を品質保証の主軸として位置付け、 全体クオリティを service-level test で担保する。 各 endpoint は service-level test を持ち、 happy path および当該 endpoint で発生しうる主要な失敗パスを service-level test でカバーする。

### endpoint の振る舞い決定 layer

各 endpoint の service-level test は **当該 endpoint の output / 副作用を直接決定する layer** に置く。 起動経路を中継するだけで output / 副作用を決定しない上位 layer では、 同じ endpoint に対する service-level test を二重化しない。

- wrapper / library が提供するシステム挙動 (例: CLI の `--version` / `--help` 等の wrapper 内蔵 endpoint) は wrapper 側の test layer に service-level test を置く。 wrapper を利用するだけの上位 layer (= caller) は wrapper の test を信頼し、 同 endpoint を再 test しない
- 上位 layer 自身が固有 endpoint (= 当該上位 layer のみが output / 副作用を決定する subcommand 等) を持つ場合、 その endpoint は当該上位 layer で service-level test を書く
- 「output / 副作用を決定する layer」 は、 stdout / stderr / 終了コード / 副作用の表面値を最終的に決める package を指す。 複数 layer を経由する dispatch は中継のみとみなす

### unit test の責務

unit test は service-level test を補完する補助層に位置付ける。 service-level test で網羅困難な部分 (純関数の境界値・組み合わせ網羅、 service-level 経路で再現困難な内部状態のエッジケース等) のみ unit test として記述する。 service-level test と重複する範囲は unit test を書かない。

### e2e test の責務

e2e test は単一 endpoint では検証できない複数 endpoint 横断の利用者シナリオ (例: コマンド A の出力をコマンド B が消費する流れ、 状態を残す endpoint 間の相互作用等) を対象とする。 単一 endpoint で完結するシナリオは service-level test で記述する。

### test の配置

- service-level test と unit test は対象実装と同じ Go module 内に配置し、 対象 package と同居する `*_test.go` で実装する
- e2e test は対象実装の package とは独立した layer に配置する。 具体配置先 / 起動経路は本 ADR の対象外とし、 e2e test を実装する時点で本 ADR を改訂して規定する

### mock を採用しない / fake を採用する

service-level test と e2e test では mock (呼び出し記録・期待値宣言中心の差し替え) を採用しない。 外部依存の置換が必要な場合は fake (本物に近い動作を持つ軽量実装。 emulator・in-memory 実装等) を採用する。

- public に利用可能な fake / emulator が存在する依存はそれを使う
- public に存在しない場合は flame 配下に fake を実装する
- unit test は対象が純関数で外部依存を持たない場合が多いため本制約の対象外。 unit test で外部依存を扱う必要が生じた場合も mock ではなく fake を使う

### test 充足度の AI レビュー

push 対象差分の test 充足度 ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md) の 1 層目 PreToolUse hook 内で起動する AI レビューの観点) を専用の AI subagent で評価する。 レビュー観点は本 ADR で定める test layer / 配置 / mock 不採用方針 / fake 採用方針への準拠。 静的検査でカバー可能な部分は別途 lint / 静的検査に寄せる ([FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md))。

### AAA パターン

各 test ケース内は **Arrange / Act / Assert** の 3 段に分け、 各段の先頭に `// Arrange` / `// Act` / `// Assert` コメントを入れて明示する。

- **Arrange**: 試験対象に与える input / 環境を組み立てる段 (mock setup の代わりに fake setup / context 生成 等)
- **Act**: 試験対象 endpoint を 1 回起動する段 (CLI の Run / 関数 1 つの呼び出し 等)
- **Assert**: Act の出力 / 戻り値 / 副作用を検証する段 (test double の verification method / assertion helper 等)

3 段が物理的にコメントで明示されることで、 各 test ケースを読む際に「どこで input を作り、 どこで対象を起動し、 どこで検証しているか」 が一目で分かる。 endpoint 起動が無い純関数 unit test 等で Act が 1 行に収まる場合でも、 3 段ラベルを付けることでケース間の構造を統一する。

### Arrange は静的に書く

Arrange 段は当該 test ケース内に **静的に直書き** する。 複数の API call をつなぐ動的な組み立てロジックを Arrange 共通化のために helper 関数に括り出さない。 重複は許容する。

- 理由: Arrange を動的 helper に括り出すと、 (1) helper 自体の実装不具合が複数 test ケースで同時に false-pass / 同時に壊れる経路を作る、 (2) caller (test ケース) は helper の中身を読みに行かないと「何が arranged されているか」 を把握できず、 ケースを 1 行で読み切れなくなる
- Arrange が動的 helper にしたくなるほど複雑なら、 試験対象の責務が大きすぎるか、 1 ケースが扱う観点が広すぎる兆候。 ケース分割や試験対象の責務分割を先に検討する
- Act 段で呼ぶ production 起動関数 (CLI の Run / handler 関数 等) や Assert 段で使う test double の verification method は本規約の対象外 (それらは production API / test double の verification API であり、 Arrange の共通化ではない)

### test helper signature

test helper 関数 / method の第一引数は `t *testing.T` ではなく `tb testing.TB` (interface) を受け取る。 `testing.TB` は `*testing.T` / `*testing.B` / `*testing.F` の共通 super-interface であり、 `Test*` だけでなく `Benchmark*` / `Fuzz*` 内からも同 helper を呼べるようにする。 production code が test helper を import する経路を作らないため、 helper を defining file は `*_test.go` (test-only build) に置くか、 production file に置く場合でも `testing.TB` を取ることで test 専用利用を明確にする。

- `tb.Helper()` は `testing.TB` interface に含まれているため、 helper が `t.Helper()` を呼ぶ既存の規約 (§AAA パターン) は signature 変更後も保持される
- caller (test ケース) が `t` (= `*testing.T`) を渡すと自動的に `testing.TB` を満たすため、 caller 側の書き方は変わらない
- service-level test の test double が提供する verification method も本 signature 規約の対象に含まれる

### test 内 identifier scope

test ファイル (`*_test.go`) 内で定義する identifier (const / var / type / 関数) は **使われる scope に最小化** して定義する。

- 1 つのテストケース (1 `Test*` 関数 / 1 `t.Run` subtest) でしか参照しない identifier は、 当該テストケースの関数 scope に local 宣言する (関数内 `const` / `var` / `type`)
- 同一 package 内の複数 テストケースで共有する identifier のみ package scope (`*_test.go` の top-level) に置く
- 「将来他 test でも使うかも」 という理由で先回り package scope に公開しない。 必要になった時点で関数 scope から package scope に昇格させる
- 「テストケース」 は `Test*` 関数または `t.Run` subtest のいずれか単位で数える。 同一 `Test*` 関数内の複数 `t.Run` subtest で共有する identifier は「複数のテストケースで共有」 に該当するため、 当該 `Test*` 関数の関数 scope に local 宣言する (package scope ではない)。 package scope は **別の `Test*` 関数間で共有する identifier のみ** に限定する

production code の §premature publishing ([FLM_APP_0007](FLM_APP_0007__go.md) §premature publishing) と同じ構造的判断を、 test code の identifier scope にも適用する。 これにより、 (1) 1 caller でしか使われない期待値定数 / 補助 type / sentinel error が package 全体に晒されてファイル top-level の視認性を下げる状態を避け、 (2) caller が 1 つに閉じている事実が宣言箇所から自明になる。

### assertion 規約

test の assertion は以下の規約に従う。

- **出力 / 戻り値全体に対して assertion を行う**: 個別 field の局所比較ではなく、 actual の出力 / 戻り値全体と expected 全体を比較する。 これにより actual の構造変更で test が壊れやすくなり、 「production 仕様の変更を test が検出する」 動作になる。 個別 field 比較では、 全く別の field が増減 / 改名されても test が緑のまま通り、 仕様変更を検出できない
- **expected を Marshal する (actual に Unmarshal しない)**: actual の文字列 / バイト列を `json.Unmarshal` して構造体に取り込んでから個別 field を比較する形は、 Unmarshal 経路自体に不具合 (例: tag 漏れ / encoder バグ) があると検出をすり抜けてしまう。 expected を Marshal して文字列化するか、 expected JSON を文字列で書いて actual 全体と比較する。 後者は production 側 encoder の出力フォーマットを test に焼き付けるが、 「production の出力契約を test に明文化する」 という意図を持つ
- **比較は `if` ではなく `assert` package (testify) を使う**: 標準ライブラリの `if got != expected { t.Fatalf(...) }` パターンを `assert.Equal` / `require.NoError` 等に揃える。 これにより diff 表示・型ごとの整形・assert/require の使い分け (`assert` は失敗時 fail だが続行、 `require` は即終了) が統一される
- **複数 channel を持つ test double は 1 メソッドで合算検証する**: test double が外部観測可能な複数 channel (stdout / stderr / 副作用書き込み先 等) を持つ場合、 verification API は全 channel の expected を 1 メソッドで一括受け取る形を採用する。 channel ごとに別 method を公開すると caller が一方の検証を呼び忘れたとき (例: stdout だけ検証して stderr に予期せぬ出力が漏れていることを見逃す) に test が緑のまま通ってしまう。 1 メソッドに合算することで 「観測 channel 全件の expected を caller が必ず指定する」 形を強制する。 本規約は **caller が同一 module 内に閉じる test double** に対して適用する。 module 境界を越えて公開される test double helper (例: 公開 lib の `FakeIO` 等で、 cli 等の依存 module から caller が来るもの) は、 caller が parity 比較 (例: shell 版と Go 版の出力同一性確認) や test ケース内動的組み立てのために channel を個別に取り出す経路を必要とするため、 per-channel inspection method (`StdoutString` / `StderrString` 等の getter) の併設を許容する。 ただし合算 verification API (`Verify`) は引き続き必ず提供し、 同一 module 内 caller / 通常の service-level test には合算経路を default として提示する
- **JSON / YAML 等価性比較を採用しない / 生文字列で全体比較する**: `assert.JSONEq` / `assert.YAMLEq` のような JSON / YAML 等価比較 helper は採用しない。 これら helper は (1) 内部で actual を Unmarshal する経路に乗せるため、 tag 漏れ等の encoder 側不具合をすり抜けてしまう、 (2) 順序・空白・型表記に寛容なため encoder が出力するフォーマット (改行・インデント) の変動を検出できない、 という不利益を持つ。 JSON / YAML 出力は raw string literal で expected を書き、 actual stdout 全体と完全一致比較 (`assert.Equal`) する。 production の出力契約 (構造 + フォーマット) を test に焼き付け、 構造変更とフォーマット変更を双方検出する

### 5 項目の整備状況

[FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md) で定める 5 項目について以下を整備する。

| 項目 | 整備方針 |
| --- | --- |
| 作成 skill | 省略 (test ファイルは Go ファイルであり Go ファイル作成 skill ([FLM_APP_0007](FLM_APP_0007__go.md)) に委譲) |
| lint | Go の lint を継承 ([FLM_APP_0007](FLM_APP_0007__go.md))。 test 関連 linter (paralleltest / thelper / tparallel / usetesting / testifylint / testableexamples) で test 固有の規律を担保する |
| build | Go の build に委譲 ([FLM_APP_0007](FLM_APP_0007__go.md))。 `*_test.go` は `go test` の compile 段階で build される |
| test | meta-test は持たない (test 自体を test する層は導入しない) |
| ADR ルール検査 skill | AI subagent (test-coverage-reviewer) を整備し、 push 直前 hook の段階 1 並列レビュー対象に追加する |

## 影響

- 各 endpoint 追加と同時に当該 endpoint を起動する service-level test を 1 件以上書く必要が生じる
- service-level test は black-box であるため、 endpoint の入出力境界を caller から明示的に与える設計 (引数 / 入出力 writer / 環境変数等の注入経路) を取る必要がある。 CLI では cobra の暗黙 default (os.Args / os.Stdout / os.Stderr) に依存せず、 引数 / writer を caller から受け取る経路を 1 つ用意する
- mock を使わないため、 fake / emulator が public に存在しない外部依存を service-level test で扱う場合は flame 配下で fake を自前実装するコストが発生する
- unit test の責務範囲が「service-level test で覆えないエッジケース」 に限定されるため、 unit test と service-level test の重複が減る代わりに、 新規実装に対して unit test を書くか書かないかの判断が都度発生する
- e2e test の物理配置先・起動経路は本 ADR で固定しない。 e2e test を実装する時点で本 ADR を改訂し、 配置先 / 起動経路 / 用いる fake の構成を規定する
- 既存の `lib/ex/` の `*_test.go` は純関数 (error wrapper) のエッジケース確認に該当するため unit test として位置付けが明確化される。 service-level test は cobra command tree を起動する経路で実装する
- 上位 layer (caller) は wrapper / library が提供するシステム挙動 (例: CLI の `--version` / `--help` 等) を service-level test に二重化しない構造になり、 caller layer の test 規模が wrapper 由来 endpoint 分だけ縮小する (例: `cli/internal/root/` の root_test は `--version` の subtest を持たない。 wrapper である `lib/clix/` 側の test layer がカバーする)
- test-coverage-reviewer は §endpoint の振る舞い決定 layer に基づき、 各 endpoint について「output / 副作用を決定する layer」 の sufficiency を判定する。 caller layer が wrapper 由来 endpoint を service-level test に持たないことを finding として返さない
- AI レビュー (`git push` 直前 PreToolUse hook) の段階 1 並列実行に test-coverage-reviewer subagent が加わるため、 push のたびに 3 reviewer (general-practices / rule-adr-sync / test-coverage) が並列起動する
- test-coverage-reviewer は本 ADR の決定 (3 layer の責務分担、 endpoint 単位の service-level test 必須、 mock 不採用 / fake 採用) を観点に違反を返す。 違反は AI ターン内で fix する。 reviewer subagent の registry (現行 reviewer / 段階配置 / 起動条件) は [FLM_ENG_0001](../engineering/FLM_ENG_0001__claude_code.md) §影響 を一次情報源とする
- service-level test を Go の同 package に置くため、 test を含む package の数が増えるたびに `go test` の対象 package が増え、 `scripts/check-go-test.sh` の 1 回の検査範囲も増える
- test 用 fake を flame 配下に持つ場合、 fake は配布対象アプリケーション ([FLM_APP_0007](FLM_APP_0007__go.md) §配置) のバイナリに含めない構造 (build tag / 別 package / 別 module) を採る必要が生じうる。 具体構成は依存が発生した時点で決める
- 「mock を採用しない」 規約は test に閉じる。 production code の依存注入や interface 抽象化までは制約しない
- Go の test 関連 linter (paralleltest / thelper / tparallel / usetesting / testifylint / testableexamples) は既に有効化されており、 本 ADR で改めて設定変更を要しない
- 本 ADR は APP カテゴリのため、 依存側プロジェクトでも本規約 (3 layer / service-level 主軸 / mock 不採用 / fake 採用 / 配置) が伝播する
- GitHub Actions ワークフローを endpoint とみなす規定 (本 ADR §endpoint の単位) のため、 各ワークフローファイルは [FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md) §test の必須化と配置 に従い対応する test script を 1 本必ず持つ。 当該 test script は ワークフローの dispatch / parse 正しさを外部観測可能な形 (uses 解決 / inputs 整合 / act parse) で検証する service-level test の具体形となる。 配置先・命名・lint との束ね方は ENG カテゴリ側 ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md)) で具体化する
- §assertion 規約 のため、 各 Go module の test は `github.com/stretchr/testify` を依存に追加する。 `cli/go.mod` に `github.com/stretchr/testify` を加え、 `go mod tidy` で `cli/go.sum` を更新する
- §assertion 規約 を機械的に検査するため、 既に有効な golangci-lint の **testifylint** linter (FLM_APP_0007 §影響 で enable 済) が testify 利用の正しさ (`assert` vs `require` の使い分け、 `EqualError` 等の正しい関数選択、 引数順 等) を検査する。 「actual に Unmarshal しない」 「出力全体を比較する」 等の意味的判断は静的化困難なため AI レビュー (test-coverage-reviewer) で補完する
- §assertion 規約 の 「JSON / YAML 等価性比較を採用しない」 規約は **forbidigo** linter で静的検出する。 `.golangci.yaml` の forbidigo.forbid に `assert.JSONEq` / `require.JSONEq` / `assert.YAMLEq` / `require.YAMLEq` の identifier pattern を登録し、 これら helper の利用を機械的に禁止する。 違反は AI ターン内で raw string 比較 (`assert.Equal`) に書き直して fix する
- §AAA パターン は完全な静的検査が困難。 「`// Arrange` / `// Act` / `// Assert` コメントが test ケース内に存在するか」 を regex で検査するカスタム静的検査は技術的には可能だが、 (1) 1 行で完結する純関数 test 等で 3 段全てを必要としないケースの false positive、 (2) コメント文字列の表記ゆれ (`// arrange` 小文字 / 全角 等) を検出側で正規化する手間、 という不利益がある。 既存の go test 関連 linter (paralleltest / thelper / tparallel / usetesting / testifylint / testableexamples) には AAA を直接強制する機能は無い。 AI レビュー (test-coverage-reviewer) で補完する
- §test helper signature は静的検査では機械検出困難。 既に有効な golangci-lint の **thelper** linter (FLM_APP_0007 §影響 で enable 済) は helper 関数の `t.Helper()` 呼び出しを検査するが、 第一引数の型 (`*testing.T` / `testing.TB` のいずれか) は検査対象に含まれない。 testify 利用検査の **testifylint** も対象外、 **revive** / **gocritic** にも該当 rule 無し。 「helper の第一引数を `testing.TB` にする」 という規約は AST 走査するカスタム静的検査として実装可能だが、 既存 linter で検出されない上、 (1) `Test*` のトップ関数 (helper でない) の `t *testing.T` を誤検出しないよう scope を絞る必要、 (2) helper 判定は thelper と同等の前段解析が必要、 という実装コストが伴う。 当面は AI レビュー (test-coverage-reviewer) で補完し、 違反パターンの累積数を見て将来カスタム linter 化を検討する (FLM_GEN_0004 §ADR rule 追加時の静的 lint 評価義務)
- §test 内 identifier scope は完全な静的検査が困難。 既に有効な golangci-lint の **unused** linter (FLM_APP_0007 §影響 で enable 済) は完全に未参照な identifier しか検出せず、 「1 関数からしか参照されていない package scope identifier を関数 scope に降格すべき」 を機械判定する linter は無い (`varcheck` / `deadcode` 等も対象外)。 [FLM_APP_0007](FLM_APP_0007__go.md) §premature publishing と同じ構造的問題で、 cross-caller 数までは AST から数えられるものの、 「将来他 test ケースでも使う予定があるか」 という意図と区別できないため static には完結しない (FLM_GEN_0004 §ADR rule 追加時の静的 lint 評価義務)。 AI レビュー (test-coverage-reviewer / general-practices-reviewer) で補完する

## 評価

代替案として以下を検討した。

### test layer の構成

- **layer を分けず単一 (unit のみ)**: 関数 1 つを単独で検証するため変更影響を局所化しやすい。 一方、 (1) endpoint を組み立てた状態の振る舞い保証が無く、 「個別関数は緑だが endpoint としては壊れている」 状態を検出できない、 (2) AI 開発前提 ([FLM_GEN_0002](../general/FLM_GEN_0002__flame.md)) では AI が小さい関数単位で繰り返し編集するため、 unit test だけだと endpoint レベルの回帰検出が AI ターン内で得られない、 という不利益がある。 service-level test を主軸として補助に unit test を置く構成を採用した。
- **layer を分けず単一 (service-level のみ)**: endpoint 全体の振る舞いを保証でき AI / 人間にとって品質指標が明快。 一方、 service-level 経路で再現困難な内部のエッジケース (純関数の境界値、 状態遷移の組み合わせ等) を網羅するための test を書くと、 service-level test として無理に endpoint 経路を経由させる test 群が増えて読みにくくなる。 service-level に重複して内部経路を再現するより、 unit test を補助層として置く方が記述コストとカバレッジのバランスが取れる。 layer を 2 層 (service-level + unit) として分担する方を採用した。
- **layer を分けず単一 (e2e のみ)**: 利用者シナリオの再現性は最大化される。 一方、 (1) e2e test は複数 endpoint 横断のセットアップ・状態管理を伴い 1 件あたりのコストと実行時間が大きい、 (2) AI ターン内 hook で頻回回したい軽量 test として向かない、 (3) 単一 endpoint の振る舞い保証も e2e で覆うと test の責務が膨らむ、 という不利益がある。 e2e を最小限に絞り、 endpoint 単位は service-level、 関数単位は unit に分担する方を採用した。
- **layer を 4 つ以上に細分化する (例: integration test を独立 layer として置く)**: 細分化で各 layer の責務はより明快になるが、 (1) layer 数が増えるたびに新規 test を書くときの判断点が増える、 (2) flame の現時点の規模では integration の独立した役割 (複数 module / 複数プロセス間の結合) を service-level / e2e のいずれかで吸収できる、 という事情がある。 3 layer (e2e / service-level / unit) に固定して整理した。

### service-level test の主軸化

- **unit test を主軸とし service-level test を補助に置く**: 関数単位での回帰検出が高速で AI ターン内 feedback に乗りやすい。 一方、 (1) endpoint レベルでの振る舞い保証が薄くなり、 個別 unit が緑でも endpoint として壊れている状態を許容してしまう、 (2) リファクタリングで関数構造を変えたとき unit test 側も同期更新が必要となり、 振る舞い不変の変更でも test 修正コストが発生しやすい、 という不利益がある。 endpoint 境界で振る舞いを固定する service-level test を主軸とする方を採用した。
- **e2e test を主軸とする**: 利用者から見える振る舞いの保証が最も強い。 一方、 e2e test の実行コスト・セットアップコストが大きく、 AI ターン内 hook で頻回回す層に置くと feedback ループが長くなる ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md))。 e2e は補助、 service-level を主軸とする方を採用した。
- **layer 間の比率 / カバレッジ比率を ADR で固定する (例: 「service-level coverage 80% 以上」 等の数値目標)**: 数値による担保は機械検査しやすい。 一方、 (1) coverage 数値はカバー対象の選定で簡単に膨張・収縮するため AI / 人間が数値合わせの test を書く誘惑が生じる、 (2) 「endpoint 単位で service-level test を持つ」 という構造的な担保の方が AI / 人間が新規実装時に判断しやすい、 という事情がある。 構造的な担保 (1 endpoint = 1+ service-level test) を採用し、 数値目標は採用しない。

### mock の扱い

- **service-level test でも mock を許容する**: mock library (gomock / testify mock 等) で外部依存を差し替えられ、 fake 実装の手間がゼロ。 一方、 (1) mock test は対象コードが「どう呼んだか」 (interface 呼び出しの記録) を検証する形になりやすく、 内部実装の構造を test に焼き付けてしまうため、 振る舞い不変のリファクタで test が壊れやすい、 (2) 実装の振る舞いではなく mock 自体の宣言した振る舞いを検証することになり、 「実装と乖離した mock が誤った緑を返す」 状態が起きうる、 (3) AI 開発前提では AI が短いサイクルで内部構造を書き換えるため、 mock 焼き付けによる test 修正コストが累積する、 という不利益がある。 service-level / e2e では mock を採用せず fake を使う方を採用した。
- **mock も fake も両方許容して都度判断**: 柔軟性は最大化する。 一方、 module 横断で test の書き方が分散し、 (1) AI / 人間が新しい test を書くときに毎回 mock vs fake を判断する必要、 (2) PR レビューで mock 焼き付けの良し悪しを毎回議論する必要、 という不利益がある。 ADR で「service-level / e2e は fake」 と固定して判断点を消した。
- **mock も全 layer 禁止 (unit test も含めて mock 不採用)**: 統一性は最も高い。 一方、 unit test は純関数を対象とすることが多く外部依存を持たない場面が多いため、 mock 禁止規約が空回りしやすい。 また unit test で稀に外部依存が出てきた場合に fake を強制すると小さいエッジ確認 1 件のために fake を実装する重さが釣り合わない場合がある。 service-level / e2e で mock を禁止し、 unit test は本制約の対象外 (使う必要が生じた場合も fake 推奨) としつつ規約上の禁止対象は service-level / e2e に限定した。
- **fake が public に存在しない場合は当該外部依存を test 対象から外す (skip / 環境変数で無効化)**: 自前 fake 実装コストは発生しない。 一方、 当該経路の振る舞い保証が test から消えるため、 「test は緑だが本番で壊れる」 状態を許容してしまう。 自前 fake 実装コストを受け入れ、 flame 配下に fake を実装する方を採用した。

### e2e test の規定範囲

- **本 ADR で e2e test の物理配置 / 起動経路まで規定する**: e2e に関する判断点を 1 ADR で集約できる。 一方、 flame の現時点で e2e test の実装対象が無く (CLI が単一 endpoint レベルで完結する範囲のため)、 規約だけ先に決めると実装段階で前提が変わって ADR 改訂が発生しやすい。 本 ADR では e2e の責務 (複数 endpoint 横断シナリオ) のみ固定し、 配置 / 起動経路は実装する時点で決める方を採用した。
- **e2e test を本 ADR から外して別 ADR (FLM_ENG_xxxx_e2e.md 等) に切る**: e2e の独立性が高まる。 一方、 3 layer は同一の test 体系内の役割分担であり 1 ADR で全体像が見える方が AI / 人間が test 戦略を読み取りやすい。 本 ADR に 3 layer 全てを集約し、 e2e の具体規定だけ将来追記する方を採用した。

### AI レビュー観点

- **test 充足度を専用 subagent ではなく既存の general-practices-reviewer で扱う**: subagent 数が増えない。 一方、 general-practices-reviewer は「業界標準的な開発プラクティス」 という flame に依存しない観点を扱うため、 flame 固有の test layer / mock 不採用方針を当該 subagent に混ぜると役割が濁る。 ADR 準拠観点 (adr-reviewer) との関係も、 adr-reviewer は ADR 全体の決定への準拠を見る汎用 reviewer であり、 test 観点を集中して見る専用 subagent を設けた方が違反検出の集中度が上がる。 専用 subagent を設ける方を採用した。
- **test 観点を adr-reviewer の中で扱う (専用 subagent を設けない)**: adr-reviewer が ADR 全体の準拠を担当しているため自然な拡張先に見える。 一方、 adr-reviewer は ADR 全件を横断的に見る汎用 reviewer であり、 1 ADR の論点 (本 ADR の test layer) を集中的に深掘りする責務を持たせると役割の粒度が不揃いになる。 test に集中する subagent を切り出す方を採用した。
- **test 充足度評価を静的検査として実装する (専用 lint plugin 等)**: 決定論的な検査で結果が安定する。 一方、 「endpoint に対応する service-level test があるか」 「mock library の import が無いか」 程度は静的化できるが、 「test が当該 endpoint の主要な失敗パスをカバーしているか」 「test の検証内容が振る舞いを意味のある粒度で見ているか」 といった意味的判断は静的化できない。 静的化できる項目は将来 lint plugin で扱い、 意味的判断を AI subagent に分担させる方を採用した ([FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md))。

### test 内 identifier scope の固定化

- **scope ルールを ADR で固定せず著者判断に委ねる**: ケースごとに最適な置き場所を選べる柔軟性が得られる。 一方、 (1) 「将来他 test でも使うかも」 で先回り package scope に置いた識別子が累積し、 ファイル top-level の視認性を落とす、 (2) AI / 人間が新規 test を書くときに毎回 scope の置き場所を判断する必要が生じる、 (3) PR レビューで scope の良し悪しを毎回議論する必要が生じる、 という不利益がある。 構造的ルール (1 caller = 関数 scope) を ADR で固定し判断点を消す方を採用した。
- **scope 規約を新規 ADR (汎用 scope rule) として切り出す**: production code と test code を横断する scope 規約として独立 ADR にできる。 一方、 production code の scope は §公開 struct の最小化 / §premature publishing で既に [FLM_APP_0007](FLM_APP_0007__go.md) に集約されており、 test 固有の scope (1 ケース = 関数 scope) は本 ADR の §test helper signature と一体で扱う方が test 戦略全体の整合が取れる。 本 ADR (FLM_APP_0009) に追記する方を採用した。
- **scope 違反を専用カスタム linter で機械検出する**: 決定論的な検査で AI レビューに依存しない。 一方、 (1) cross-caller 数の判定までは静的化できるが「将来 caller を増やす意図があるか」 と区別できない、 (2) 既存 `unused` / `varcheck` / `deadcode` のいずれも完全未使用しか対象としない、 (3) 専用 linter 実装コストに対して [FLM_APP_0007](FLM_APP_0007__go.md) §premature publishing で既に同種の判定を AI レビュー側に委ねており、 同じ補完経路に乗せる方が運用が単一化する、 という事情がある。 AI レビュー (test-coverage-reviewer / general-practices-reviewer) で補完する方を採用した ([FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md))。

### endpoint の振る舞い決定 layer の固定化

- **全 layer で service-level test を二重化する**: 同一 endpoint について複数 layer (wrapper + caller) で service-level test を持つ。 wrapper 仕様変更時に caller test も同期修正する必要が累積し、 修正コストが layer 数に比例する。 また、 caller test が wrapper の振る舞いを再確認する形になり、 caller の責務 (subcommand の組み立て・登録 等) を test する観点と混在する。 振る舞いを決定する layer に test を集約する方を採用した。
- **endpoint の責務 layer を ADR で固定せず著者判断に委ねる**: layer 設計の柔軟性は最大化されるが、 (1) 同じ endpoint について caller layer と wrapper layer のどちらに test を書くかが PR ごとに揺れる、 (2) test-coverage-reviewer が sufficiency を判定する基準も都度判断になり、 review 結果が安定しない。 振る舞い決定 layer に固定する方を採用した。
