# Go を主開発言語として採用する

## 背景

- flame は新規アプリケーション開発の DX を改善する framework である ([FLM_GEN_0002](../general/FLM_GEN_0002__flame.md))
- flame は AI エージェントとの協働開発を前提として設計する ([FLM_GEN_0002](../general/FLM_GEN_0002__flame.md))
- flame の自動化処理 (hook / CI 検査スクリプト等) は shell スクリプトで実装している ([FLM_APP_0002](FLM_APP_0002__shell_script.md))
- flame は今後 CLI ツール、および web server を提供する計画があり、その実装言語が定まっていない
- Go は静的型付けでコンパイル時に型検査が走り、シングルバイナリ生成・cross-compile が toolchain 標準で完結する
- Go には公式 toolchain (`go build` / `go test` / `go vet` / `gofmt`) が同梱され、依存解決機構 (`go mod`) も公式提供されている
- Go module は `go.mod` を持つディレクトリ単位で独立した依存解決単位として扱われる
- flame の静的検査は checker を単位として組み立て、 1 種別あたり 1+ checker を許容する。 各 checker は trigger (起動契機の blast radius) と target (1 起動の execution unit) を独立に持つ ([FLM_FEA_0001](../feature/FLM_FEA_0001__checker.md))
- Go module は内部に複数の package を持ちうる (lib package、 `main` package = ビルド対象バイナリ、 test 含む package など)。 公式 toolchain は次の粒度で動作する: gofmt = ファイル / ディレクトリ、 go vet / go test = package、 go build = main package、 go mod tidy = module 単位
- Go の build / test は依存解決を介して module 内の他 package へ影響が波及するため、 1 ファイル変更でも当該 module 内の全 main package の build と全 test 含む package の test を再実行する必要がある (依存波及範囲を classifier 側で個別に算出する手段を持たないため、 安全側に倒す)
- gofmt 等の format 系 / vet 等の lint 系は副作用なしで個別 file / package に閉じる検査だが、 lint カテゴリ全体としては build / test 以外の Go 関連検査 (`go mod tidy` の差分、 module-wide 整合性検査等) を含める必要がある
- Go の静的検査エコシステムには公式 toolchain (`gofmt` / `go vet`) に加え、 多数の linter (errcheck / staticcheck / errorlint / gosec / gocritic / revive 等) を 1 つの設定ファイルから統合実行する meta linter (golangci-lint 等) が広く採用されている。 個別 linter を都度導入するよりも meta linter で一括管理する方が、 採用 linter リスト / 設定 / 実行コストの管理が一元化される
- 上記 meta linter は default の ruleset (enable される linter 集合) を持つが、 該当 default はバージョンアップで変更されることがある。 default に依存すると flame の検査契約 (どの種類の違反を fail とするか) が暗黙に変わる懸念がある
- service-level の重い test を Go module 全体で 1 度に走らせると CI 時間が膨らむため、 chunk 単位 (build tag、 test 名集合、 shard 等) での分割実行が将来必要になる
- flame ではコンテンツ種別ごとに「作成 skill / lint / build / test / ADR ルール検査 skill」の 5 項目を整備する規約がある ([FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md))
- flame は静的にチェックできるルールを静的チェックで担保する方針である ([FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md))
- flame の開発環境マネージャは devbox であり、ローカル開発では direnv 経由の自動アクティベーションで devbox 管理下のツールを直接呼び出す ([FLM_ENG_0002](../engineering/FLM_ENG_0002__devbox.md))

## 決定

flame では Go ファイルを以下のルールで扱う。

### 主開発言語

- flame が新規に提供するアプリケーション (CLI ツール、web server 等) は Go で実装する
- shell スクリプトで実装している既存の自動化処理 ([FLM_APP_0002](FLM_APP_0002__shell_script.md)) は当面そのまま維持する。Go への置き換えは個別判断とする

### 配置

- Go アプリケーション / パッケージは独立した module (`go.mod` を持つディレクトリ) 単位で配置する
- 各 module はリポジトリルート直下のディレクトリ (`<module>/`) に置き、module 内で完結させる (build / test の実行が他 module に依存しない)
- 各 module の main package は `<module>/cmd/<app_dir>/main.go` に配置する。 module root 直下に main package を置かない
- `<app_dir>` の命名は次の規約に従う:
  - 配布可能なアプリケーション (リポジトリ外部のユーザに配布する CLI / daemon 等) は `<app_dir>` を `<app_name>_tool` とする (suffix `_tool` 付き)
  - 配布を前提としないアプリケーション (リポジトリ内検査用ツール、 開発支援用の内部バイナリ等) は `<app_dir>` を `<app_name>` とする (suffix なし)
- アプリケーション名 (= バイナリ名 / コマンド名) は `<app_dir>` から `_tool` suffix を除いた `<app_name>` とする (suffix の有無は配布可否を表すマーカーであり、 アプリケーション名そのものには含めない)
- 1 module 内に main package を複数配置する場合は、 各 main package が独立した `<app_dir>` を持つ
- module ディレクトリ名 (= リポジトリルート直下、 `go.mod` を持つディレクトリ名) は配布対象 library module の marker を兼ねる:
  - 配布対象 library module (Go module として外部から `go get` で取得される対象) は module ディレクトリ名を `lib` または `<name>_lib` (suffix `_lib` 付き) とする
  - 配布対象 library module でない module は `lib` / `_lib` suffix を持たない自由名 (例: `cli/`) とする
- 配布対象 library module marker (module 名) と配布対象 main package marker (`<app_dir>` の `_tool` suffix) は直交に評価する。 1 module が両方に該当する形 (例: `<name>_lib/cmd/<demo>_tool/`) も許容する

### package 命名

- package 名 (= ディレクトリ名 + 内部 `package <name>` 宣言) は **snake_case** を使い、 複数単語を `_` で区切る (例: `flow_document`、 `github_actions`、 `pre_push`)。 Go community 慣習 (all-lowercase, no underscores) ではなく snake_case を採用する
- 派生する規約:
  - CLI subcommand 名がハイフン区切り (例: `flow-document`) の場合、 対応する subcommand package のディレクトリ名・package 名はハイフンを `_` に置換した snake_case (例: `flow_document/`) とする
  - `_test` suffix は test ファイル / black-box test package の慣用で、 package 名としては衝突しないように複数単語の最終要素として使わない
- Go 予約語と衝突する名前 (例: `go` / `func`) は別の名前 (例: `golang`) を選ぶ

### コメント

- Go ソース内のコメントはドキュメント・コメントの自然言語規約 ([FLM_APP_0001](FLM_APP_0001__document.md)) を継承する

### go.mod directives

flame では Go module の依存解決を上流リポジトリの公式版 (= module path で指す release) に統一する方針を採用する。 これに伴い `go.mod` の以下 directive を禁止する。

- `replace` directive は使用しない (local path 形式・remote module 形式のいずれも禁止)
- fork が必要な依存は upstream に PR を送るか、 import path を fork 側 (= 別 module path) に切り替えて `require` directive で正面から指す

### 公開 struct の最小化

flame では Go の公開 API における struct の露出を最小化する方針を採用する。 これを実現するために以下を定める。

- library / wrapper package が定義する struct 型 / option 型は **全て package private** とする (大文字始まりにしない)
- caller は package private な型を名前で参照できないため、 値は **constructor 関数 (exported) の戻り値を型推論で受け取り method 呼び出しのみで使う**
- 上記 struct の構築は **constructor 関数 + functional options pattern** で行う。 constructor は必須引数を positional に取り、 任意項目は `func(*<config>)` 型の option を可変長引数で受ける
- option setter (exported 関数) の名前は `With<対象><項目名>` 形式 (例: `WithRootShort`) にする
- exported function が package private な型を返すパターン (option pattern の不可避要件) を許容する
- 上記の制約により package private 型を関数 signature で名前参照できなくなるため、 cross-package で値を直接受け渡す API ではなく、 **chain 化した即時利用** (例: `clix.NewRoot(...).Execute()`) もしくは facade 関数 (例: `root.Execute()`) を経由させる
- 例外として、 test 用 double (fake / stub / spy 等) struct は public にしてよい。 test caller 側で型を named variable で受けて verification method (`VerifyXxx` 等) を呼ぶ用途があり、 また test 専用のため production API surface には載らない。 命名は test 用語を含む形 (`FakeXxx` / `StubXxx` / `SpyXxx`) を用いる ([FLM_APP_0009](FLM_APP_0009__test.md) §test double 命名 と同じ規約)
- 例外として、 純粋な data transfer object (JSON / YAML 等の serialize 対象、 protocol message 等) は zero value 利用と field 追加の前方互換性が成立しやすいため、 本規約の対象外とする

### error 表現と stacktrace

Go コードで返す error には stacktrace を必ず付与する方針を採用する。

- error 生成 / wrap は flame 内の error wrapper package のみを介する。 wrapper は配布対象 library module (= §配置 で定める module 名 `lib` / `_lib` suffix) の export root 直下 (`<lib_module>/ex/`) に配置する。 非 lib module は自身に wrapper を持たず、 当該 library module の wrapper を import して利用する
- error wrapper は標準ライブラリのみで実装し、 3rd party の error library には依存しない (基盤 utility なので flame 配下で完結させる)
- 外部 package 由来の error をそのまま返さない。 関数境界を越える際は wrapper の Wrap / Wrapf で stacktrace を付与する
- error wrapper の utility そのものは前述 §公開 struct の最小化 の対象外 (関数 API のみで struct を持たない)

### premature publishing

flame の Go コードでは「将来公開するかも」 という理由で先回り publish しない方針を採る。

- 同一 package 外から利用される必要がない名前 (関数 / 型 / 定数 / 変数 / interface / interface methods) は **private** (小文字始まり) にする
- 「将来別 package から使うかもしれない」 という想定だけで public 化しない。 実際に cross-package で必要になった時点で private → public に昇格する
- interface も同様。 interface 自体は cross-package boundary で使うために public 化する場合があるが、 method 集合の各 method は「同じ package 内の実装が満たすために存在する」 意図でのみ public/private を選ぶ。 caller package がその method を直接呼ぶ必要が無い場合は method を private にする (sealed interface パターン)。 これにより interface を満足できるのは同一 package 内の型に限定され、 type 階層の閉鎖性が compile-time に保証される

### 戻り値型

flame の Go コードでは「accept interfaces, return concrete types」 という Go community の慣習に従う。

- 公開関数 / method の戻り値は **interface ではなく実体 (concrete type)** を返す
- caller 側で 「この値は特定の interface を満たしている」 ことに依存させたい場合、 製造者側 package 内に `var _ Iface = (*Concrete)(nil)` の compile-time assertion を 1 行置き、 戻り値型は実体のまま保つ
- 例外として、 戻り値が「複数の concrete 型のうちどれかを返す」 動的選択を持つ場合は interface 戻り値を許容する
- 別の例外として、 §公開 struct の最小化 で private 化された struct を sealed interface (package private method を持つ interface) 経由で cross-package に受け渡す必要がある場合 (= 当該 interface を満たせる concrete 型は wrapper package 内部の private struct のみ、 caller は値の引き回しのみ行い method を直接呼ばない) は、 関数戻り値を当該 sealed interface とすることを許容する

### test double 命名

production の代替実装 (test double: fake / stub / spy / mock 等) には test 用語を含む名前を付ける。

- 命名は test double の種類に対応する prefix / suffix を含める (例: `Fake<Name>` / `<Name>Fake` / `Stub<Name>` / `Spy<Name>`)。 flame では mock を採用しないため (FLM_APP_0009 §mock を採用しない / fake を採用する)、 採用される命名は主に `Fake` 系
- caller (test code) が production 実装と test double を見分けられるようにするため、 「Test」 等の汎用語ではなく 「Fake」 等の test double 種別を明示する語を使う

### context 伝搬

flame の Go コードでは context.Context を関数境界で明示的に伝搬し、 起動から個別 IO / IPC / 外部依存呼び出しまでの cancel / deadline / 値伝搬を一貫させる。

- main 関数は起動時の root context を生成する責務を持つ。 最小では `context.Background()` を起点とし、 SIGINT / SIGTERM 等の signal 駆動 cancel が必要になった段階で `signal.NotifyContext` 等で派生 ctx に置き換える
- module 横断境界 (lib package の exported 関数、 別 package から呼ばれる exported 関数、 cobra RunE 相当の subcommand 実行関数) は **第一引数に `context.Context`** を取る (Go community 慣習に従う)
- 例外として副作用を持たない pure utility (例: `ex.Wrap` / `ex.Wrapf` / `ex.New` / `ex.Errorf` のような stacktrace 付与のみを行う関数) は ctx を取らない。 IO / IPC / 外部依存呼び出しを含みうる関数は carve-out 対象外とし、 必ず ctx を取る
- cobra wrapper の root command が ctx を受け取り、 cobra tree 全体に伝搬する。 subcommand 実装は cobra runtime 経由で ctx を受け取る

### checker 構成

Go 種別の checker は flame の checker 規約 ([FLM_FEA_0001](../feature/FLM_FEA_0001__checker.md)) に従って構成する。 抽象 policy として以下を定める。

- Go 種別は **lint / build / test の 3 operation** に分け、 各 operation を独立 checker として実装する
- 全 checker の trigger = **module**、 target = **package** とする
- **lint** カテゴリは build / test 以外の Go 関連検査 (format、 vet、 module manifest 整合性、 community 標準 lint ルール群等) を一括して扱う。 個別 linter を都度導入する代わりに **多 linter 統合 meta linter** を主軸とする
- meta linter の運用ポリシー: **default の ruleset を使わず、 採用する linter を全て明示的に enable する**。 これにより meta linter のバージョンアップで default が変動しても flame の検査契約は不変に保たれる。 同じ理由で default の exclusion preset / 内蔵 issue 抑制も使わず、 必要な例外は明示する
- meta linter の構成は **厳格寄り** (community で広く採用される strict セット) を採用する
- **build** は main package のみを対象とする (1 module 内に main package が複数あればそれぞれ独立に検査する)
- **test** は test を含む package のみを対象とする (chunk 単位への細分化は将来要件として保留)

mode (fix / diagnose、 [FLM_FEA_0001](../feature/FLM_FEA_0001__checker.md) §起動モード) の運用:

- **lint** checker のみ fix / diagnose で挙動が分かれる (fix で auto-fix を適用、 diagnose で読み取り)
- **build / test** は両 mode で同動作 (auto-fix 概念を持たない検査のため [FLM_FEA_0001](../feature/FLM_FEA_0001__checker.md) の degenerate ケース)

trigger / target の運用 ([FLM_FEA_0001](../feature/FLM_FEA_0001__checker.md) §trigger と target の分離):

- 変更ファイルを契機に classifier が所属 module を解決し、 影響 module 集合を作る
- 各 module の package 構造を classifier が scan し、 lint / build / test 各 checker の target に enumerate する
- 1 ファイル変更でも当該 module 内の全関連 package が target になる (依存波及を classifier 側で解析する手段を持たないため、 module-wide trigger で安全側に倒す)

### 5 項目の整備状況

[FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md) で定める 5 項目について以下を整備する。

| 項目 | 整備方針 |
| --- | --- |
| 作成 skill | 整備 |
| lint | 整備 |
| build | 整備 |
| test | 整備 |
| ADR ルール検査 skill | 省略 (lint で完結する範囲に限定) |

## 影響

- flame で扱う言語が shell に加えて Go の 2 種類になる
- devbox に Go toolchain (`go`) を追加するため `devbox.json` / `devbox.lock` のメンテナンス対象が増える ([FLM_ENG_0002](../engineering/FLM_ENG_0002__devbox.md))
- Go ソースファイルの拡張子 (`.go`) と文字エンコーディング (UTF-8) は Go toolchain 仕様で固定されるため flame 側で別途規定する余地はない
- リポジトリ root 直下に Go module ディレクトリ (`cli/` 等) が並ぶ構造になる
- 各 module 内の main package は `<module>/cmd/<app_dir>/main.go` という固定階層で見つかる。 1 module 内に main package が複数あれば `<module>/cmd/` 配下に並列に並ぶ
- `<app_dir>` の `_tool` suffix の有無により、 ディレクトリ一覧から当該 main package が配布対象かを機械的に判別できる
- module ディレクトリ名 (`lib` / `<name>_lib`) の有無により、 リポジトリルート直下のディレクトリ一覧から当該 module が配布対象 library かを機械的に判別できる (`^(lib|.*_lib)$` の正規表現で 1 行判別)
- 配布対象 library と配布対象 tool の marker は独立に評価されるため、 1 module 内に library 配布対象と tool 配布対象が併存する形 (`<name>_lib/cmd/<demo>_tool/` 等) でも判別が壊れない
- 配布対象 library module の release 経路 (tag 名・asset 形式・install 経路) は [FLM_FEA_0004](../feature/FLM_FEA_0004__release_policy.md) に従う
- アプリケーション名 (バイナリ名) と `<app_dir>` 名がずれる (suffix 付きディレクトリでは `flame_tool/` → `flame`)。 build / test / 配布スクリプトが `<app_dir>` から `_tool` suffix を除いたバイナリ名を導出する規約を共有する
- 採用ルールに従い、 Go 種別 checker は `scripts/check-go-lint.sh` / `scripts/check-go-build.sh` / `scripts/check-go-test.sh` の 3 つに分割される。 lint checker は **golangci-lint** (gofmt / govet / staticcheck / errorlint / gosec / gocritic / revive 等を統合した meta linter) と `go mod tidy -diff` (manifest 整合性) を担い、 build checker は `go build` で main package を、 test checker は `go test` で test 含む package を対象とする
- golangci-lint の設定はリポジトリルートの `.golangci.yaml` に置く。 default の linter ruleset / exclusion preset / generated 緩和は使わず、 採用 linter / 設定を明示列挙する。 これにより golangci-lint のバージョンアップで default が変動しても flame の検査契約は不変に保たれる
- golangci-lint の auto-fix capability を持つ linter / formatter (gofmt, gofumpt, goimports, errorlint の一部 等) は fix mode で `--fix` 起動により hook 上で自動適用される ([FLM_FEA_0001](../feature/FLM_FEA_0001__checker.md) §起動モード)。 残違反は AI / 人間が意味的に解消する
- 1 ファイル変更を契機に当該 module 内の全 package (lint)、 全 main package (build)、 全 test 含む package (test) が target として enumerate される。 検査時間は module 内の package / main / test 数に比例する一方、 package を跨ぐ依存影響 (例: `internal/foo` 変更による `cmd/server` の build 失敗) は確実に検出される
- main package が複数ある module では build が main package の数だけ実行される。 hook 層では checker 内で順次、 CI 層では `wf__check.yaml` の matrix が checker (lint / build / test) 単位で並列化するため build / test / lint は並列実行される (1 checker 内の package 順次実行は今後 chunk 化等で更に細分化可能)
- service-level の重い test を抱える module が増えた場合、 現状は package 単位の順次実行で test 並列化が頭打ちになる。 [FLM_FEA_0001](../feature/FLM_FEA_0001__checker.md) の規約に従って test chunk 単位 target への細分化 (build tag / test 名 shard 等) を別実装で追加可能
- classifier (`scripts/detect.sh`) が module 内の package を scan するため、 Go module 構造の判定処理が detect.sh に入る。 当面は go toolchain を呼び出さず find + grep でディレクトリ走査と main package 判定を行う (Go toolchain 依存の重い enumerate `go list` への移行は build tag 解釈等が必要になった時点で再評価)
- 作成 skill (`.claude/skills/go/SKILL.md`) は build / test の動作確認まで完了させる procedural を提供する
- hook 層が fix mode で lint checker を起動するため、 AI / 人間が `golangci-lint run --fix` や `go mod tidy` を手作業で実行せずに済む。 hook が静かに `cli/*.go` 等の format を適用したり `go.sum` を再生成したりするため、 同 module 内の touch していなかったファイルにも影響が出る ([FLM_FEA_0001](../feature/FLM_FEA_0001__checker.md) §起動モードの影響)
- CI 層は diagnose mode 固定のため、 PR が format 違反 / 未 tidy な manifest / lint 違反を含んでいた場合は CI fail として顕在化する。 PR 著者は手元で hook fix を走らせて (もしくは手動で `golangci-lint run --fix` / `go mod tidy` を実行して) 修正する
- Go ソース内のコメントもドキュメント自然言語規約 ([FLM_APP_0001](FLM_APP_0001__document.md)) の対象となるため、godoc 慣習 (英語で識別子の説明を書くスタイル) と衝突しうる
- 各 module は `go.mod` と (依存があれば) `go.sum` を保持し、依存追加・更新がコミット差分として残る
- module ごとに独立した `go.mod` 配置のため、複数 module で同じ依存を持つ場合は重複解決が発生する
- §go.mod directives を機械的に検査するため、 golangci-lint の `gomoddirectives` linter を有効化する。 `replace-local: false` (local path 形式の `replace` を禁止) と `replace-allow-list: []` (remote module 形式の `replace` も例外なく禁止) を設定し、 `replace` directive を全面禁止する
- 依存側プロジェクトにも本 ADR の規約が伝播する (本 ADR は APP カテゴリのため)。「flame の主開発言語として Go を採用する」決定は flame 側のアプリケーション提供方針であり、依存側プロジェクトに Go 採用を強制するものではない (依存側で Go を採用する場合に本 ADR の規約に従う、という形で伝播する)
- §公開 struct の最小化 を機械的に検査するため、 golangci-lint の `exhaustruct` linter を有効にし、 全 struct を対象に literal の field 網羅を強制する (include / exclude filter は使わない)。 外部 package の多 field struct (cobra.Command 等) は flame 内に「全 export field を zero value で明示初期化する中央 constructor」 を 1 つだけ用意し、 他の利用箇所はその constructor を呼ぶ形に統一する
- exhaustruct を回避できる bypass-pattern (`new(T)` で zero value を取得する書き方) を防ぐため、 golangci-lint の `forbidigo` linter で `new` identifier を禁止する。 構築は struct literal (全 field 明示) または constructor 関数 経由に統一される
- §公開 struct の最小化 のため、 各 module の wrapper / library package は struct / option 型を全て package private とし、 公開 API は constructor + option setter のみで構成される。 caller は型を名前で参照できないため、 exported function の戻り値を chain で即時利用するか facade 関数を経由する形に統一される
- 同方針により revive の `unexported-return` rule は flame では無効化される。 revive default の他 rule (exported / package-comments 等) は維持される
- §公開 struct の最小化 の test double carve-out により、 wrapper package 内に `*FakeIO` 等の exported test fake struct を持てる。 production 用途の builder / config / wrapper 型 (rootConfig / commandConfig / 各 With* option 等) は引き続き private 規約の対象。 carve-out 対象は test ファイル / test 利用専用の struct に限定する
- §error 表現と stacktrace のため、 flame には error wrapper package (`<lib_module>/ex/`) を 1 つ持つ。 wrapper は標準ライブラリ (`runtime.Callers` / `errors` / `fmt`) のみで実装され、 配布対象 library module (= module 名 `lib` / `_lib` suffix) の export root 直下に配置する。 非 lib module は自身に wrapper を持たず、 当該 library module の wrapper を import して利用する
- §error 表現と stacktrace を機械的に検査するため、 golangci-lint の `wrapcheck` linter を有効にし、 ex package の Wrap / Wrapf / New / Errorf を ignore-sigs に登録する。 cross-package boundary では外部 / 内部 package を問わず明示的に ex.Wrap を介する規約とし、 wrapcheck の `ignore-package-globs` は使わない (flame 内 package を一括除外すると外部 error の握り潰しを検出できなくなるため)。 ex.Wrap は idempotent で、 既に stacktrace を持つ error の再 wrap は no-op になる
- §context 伝搬 のため、 main / cobra wrapper / cross-package boundary の関数は第一引数に `context.Context` を取る規約に統一される。 main が `context.Background()` を起点に root context を生成し、 cobra wrapper の root command が `SetContext` 経由で cobra tree 全体に ctx を伝搬する。 RunE / subcommand 実装は `*cobra.Command.Context()` で ctx を受け取る。 ex package のような副作用を持たない pure utility は本規約の carve-out 対象として ctx を取らないが、 IO / IPC / 外部依存呼び出しを含む関数は必ず ctx を取る
- service-level test (FLM_APP_0009) は `t.Context()` (Go 1.24+) を第一引数に渡す経路で root を駆動する。 test 終了時に test framework が ctx を cancel するため、 test 内で leak した goroutine / 外部呼び出しが ctx 経由で停止される
- §戻り値型 を機械的に検査するため、 golangci-lint の `ireturn` linter を有効化する。 default allow-list (`anon` / `error` / `empty` / `generic` / `stdlib`) を維持し、 標準ライブラリ慣習 (`error` / `io.Reader` 等) と integral な generic / 匿名 interface は許容する。 flame 内で定義した interface (例: `clix.IO`) を戻り値に出している箇所は本 linter が検出するため、 §戻り値型 の compile-time assertion (`var _ Iface = (*Concrete)(nil)`) パターンに移行する
- §premature publishing は完全な静的検査が困難なため、 AI レビュー (general-practices-reviewer / test-coverage-reviewer 等) で補完する。 `unused` linter は「どこからも参照されていない」 名前を検出するが、 「同じ package 内からしか参照されていない」 を機械判定する linter は無く、 cross-package import を検査する側面では `revive` の `exported` rule 等が部分的に検出可能だが、 「将来公開するかも」 という意図を排除する判定は実行時 caller が決まる前は不可能
- §test double 命名 は命名規約のため lint 化が困難。 具体的には「ある型が test double であるか」 の判定自体を機械的に行う手段が無い (test 用 file `*_test.go` に置かれているか、 fake/stub という単語が含まれるか、 等の表面的特徴は検出できるが、 production 実装と test double の区別は意味的判断)。 AI レビュー (test-coverage-reviewer) で補完する
- §package 命名 採用に伴い、 multi-word な subcommand package のディレクトリ名・package 宣言は全て snake_case (例: `flow_document/`、 `pre_push/`) になる。 CLI subcommand 名 (`flow-document` / `pre-push` 等) と package 名 (= ハイフンを `_` に置換した形) が機械的に 1:1 対応する
- Go 予約語と衝突する subcommand 名 (例: `go`) は別の package 名 (例: `golang/`) を選ぶため、 subcommand 名と package 名がずれる carve-out が発生する。 caller (例: `check.New()` 内の AddSubcommand) は subcommand 名 (`go`) と package 名 (`golang`) の対応を import 行と `clix.NewCommand("go", ...)` で個別に指定する
- §package 命名 採用に伴い、 staticcheck の `ST1003` (package 名 underscore 禁止) は flame の規約と衝突する。 §lint (FLM_APP_0010) 「本決定と本質的に衝突する lint rule は無効化する」 経路に沿って `.golangci.yaml` 側で `ST1003` を staticcheck checks から除外する
- errcheck の `check-blank: true` (default) は `var _ I = (*S)(nil)` 形式の interface 充足 compile-time assertion を function call の blank 代入として誤検知する。 §戻り値型 で sealed interface 例外条項とともに当該 assertion を採用しているため、 [FLM_GEN_0006](../general/FLM_GEN_0006__no_lint_suppression.md) §グローバル無効化 経路に沿って `.golangci.yaml` 側で `errcheck.check-blank: false` を設定し、 file 単位の `//nolint` を不要にする
- errcheck はライブラリ慣習で error を返さない扱いとされる関数 (例: in-memory writer への書き込み `fmt.Fprint` / `fmt.Fprintf` / `fmt.Fprintln` / `io.WriteString`) も検出対象に含む。 これら関数は `fmt.Formatter` 実装等で `fmt.State` / `strings.Builder` 等の writer に書き込む際に error を握り潰すのが慣習で、 個別の `//nolint` を多用する事態を避けるため [FLM_GEN_0006](../general/FLM_GEN_0006__no_lint_suppression.md) §グローバル無効化 経路で `errcheck.exclude-functions` に当該 4 関数を登録する
- revive の `context-as-argument` rule は context.Context が関数の第一引数であることを要求するが、 [FLM_APP_0009](FLM_APP_0009__test.md) §test helper signature が test helper の第一引数を `tb testing.TB` (= `*testing.T` / `*testing.B` / `*testing.F`) と固定するため、 service-level test で「testing.TB → context.Context」 の順序が必要になり当該 rule と衝突する。 `.golangci.yaml` 側で `context-as-argument` の `allowTypesBefore` に `*testing.T,*testing.B,*testing.F,testing.TB` を登録し、 testing.TB 系の型は ctx の前置を許す
- §戻り値型 に追加した sealed interface 例外条項により、 wrapper / library package の private struct を cross-package で受け渡す経路が確保される。 一方 ireturn linter は当該 interface 戻り値を「公開関数の戻り値が interface である」 として検出するため、 sealed interface 戻り値を広範に採用する API では allow-list 個別登録での運用が現実的でなく、 ireturn 自体を無効化することになる (FLM_APP_0010 §lint 経路)。 sealed interface 例外条項は AI レビュー (general-practices-reviewer / adr-reviewer) で正当性を確認する

## 評価

代替案として以下を検討した。

- **TypeScript / Node.js を採用する**: web server / CLI のいずれも実装可能で AI / 開発者双方に普及している。一方、シングルバイナリ配布が標準で完結せず、静的型と実行ランタイムが分離している (tsc → node) ため、配布形態が複雑になる。flame は新規アプリ開発の出発点として配布しやすさを重視する観点から不採用。
- **Rust を採用する**: 静的型・シングルバイナリ・並行性のいずれも強力だが、所有権モデルの学習コストが高く、AI が短いサイクルで生成・修正する flame の前提 ([FLM_GEN_0002](../general/FLM_GEN_0002__flame.md)) に対して syntax / borrow checker の摩擦が大きい。
- **Python を採用する**: hello world CLI / web server とも実装容易だが、flame は Python ランタイムを採用しない方針 ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md) の評価で言及済み)。Python ランタイム導入は既存方針と整合しないため不採用。
- **shell スクリプトを拡張して CLI / web server も shell で書く**: 既存の自動化処理と言語が揃う一方、shell は型を持たず大規模化に耐えない。web server のような長寿命プロセスを安全に書く言語標準機構が無く、fmt / vet 相当の言語標準静的検査も提供されない。
- **lint カテゴリを公式 toolchain (gofmt + go vet) のみで構成する (golangci-lint を採用しない)**: 採用 linter / config の管理コストがゼロで導入が最小。 一方、 errcheck / staticcheck / errorlint / gosec / gocritic / revive 等の community 広く採用される lint ルール群を欠き、 AI 開発 harness ([FLM_ENG_0001](../engineering/FLM_ENG_0001__claude_code.md)) として「機械的に解ける問題を AI ターン内で検出 / fix する」効果が薄い。 多 linter 統合の meta linter (golangci-lint) を採用し、 AI / 人間のターン内修正サイクルに乗せられる検出力を確保する方を採用した。
- **golangci-lint の default ruleset (`default: standard` 等) をそのまま使う**: 採用 linter リストを書かずに済む。 一方、 golangci-lint のバージョンアップで default 集合は変動するため、 flame の検査契約 (どの種類の違反を fail とするか) が暗黙に変わる。 `default: none` で全 linter を明示列挙し、 設定もバージョン非依存にする方を採用した。 同じ理由で default の exclusion preset / 内蔵 issue 緩和 / generated-code 緩和も使わず、 必要な例外は config で明示する。
- **golangci-lint の linter 選定をミニマル (default 程度) にとどめる**: noise を抑える利点はあるが、 AI 開発 harness では「lint で検出 → 機械的 fix」のサイクルを多くの種類の違反に効かせる方が価値が高い。 community で広く採用される strict セット (govet / errcheck / staticcheck / errorlint / gosec / gocritic / revive 等) を enable する方を採用した。 個別 linter の追加・除去は `.golangci.yaml` の差分として明示される。
- **Go module を 1 つ (リポジトリルートに `go.mod`) に統一する**: 複数 module 間で依存解決を共有でき同一依存の重複を排除できる。一方、CLI と web server で互換性のない依存を使いたい場合や、依存側プロジェクトが flame の一部 module だけを取り込みたい場合に分離コストが発生する。module ごとに独立した `go.mod` を持つ方を採用し、必要になれば `go.work` で統合する。
- **module ディレクトリをリポジトリルート以外 (例: `apps/cli/`、`cmd/cli/`) に置く**: module 数が増えた場合に整理可能だが、現時点で module 数は少なく、ルート直下のディレクトリで物理的に分離する方が AI / 人間ともにファイルツリー全体を一望できる。module 数が増えた段階で再評価する。
- **Go 種別を Go ソース (`*.go`) と Go module manifest (`go.mod` / `go.sum`) で別 checker に分割する**: それぞれに固有の検査 (gofmt / go vet vs go mod tidy 等) があり責務分離の見方もできる。 一方、 manifest 変更は build / test / lint の全 operation の trigger となるため、 入力ファイル種別での checker 分割は意味的に重複する。 operation 軸 (lint / build / test) で checker を分け、 manifest を含めた lint カテゴリで `go mod tidy -diff` を扱う方を採用した。
- **Go 種別の target 粒度を file にする (target = ファイルパス)**: 変更ファイル数に比例して検査範囲が決まる利点はあるが、 `go vet` / `go test` / `go build` 等 Go の主要 toolchain は package / module を解決単位とするため、 file-unit 入力を受けた checker は内部で「ファイル → 所属 package の再解決」を毎回実行することになる。 ファイル → package 解決を classifier ([FLM_FEA_0001](../feature/FLM_FEA_0001__checker.md)) に集約し、 target は常に package に揃える方を採用した。
- **Go 種別の target 粒度を module にする (target = `go.mod` を持つディレクトリ)**: 1 種別 1 target で classifier / checker 双方が単純になる利点があり、 module 内整合性を 1 invocation でカバーできる。 一方、 main package が複数ある module で build を 1 invocation 内の順次実行に固定してしまい、 重い test も module まるごと 1 単位になって細分化余地が無くなる。 target 粒度を package に下げ、 lint / build / test の各 checker が package 単位の target を順次処理する形を採用した (将来 test chunk 等への細分化余地を残す)。
- **Go 種別を 1 つの checker (`check-go.sh`) に集約し internal で operation を順次実行する**: スクリプト数が少なく済む利点がある。 一方、 (1) CI matrix での operation 並列化が得られず lint / build / test 全部が 1 matrix entry 内で順次実行される、 (2) checker 内部で operation 分岐ロジックが膨れる、 (3) operation の 1 つだけ追加したい (e.g., `go mod tidy -diff` を別カテゴリ化) ときに大きな checker への変更となる、 という不利益がある。 checker を operation 別 (lint / build / test) に 3 分割する方を採用した。
- **operation を更に細分化する (例: lint を `gofmt` / `go vet` / `go mod tidy` で別 checker)**: より細かい並列化が得られるが、 (1) checker 数が膨らみメンテナンス対象が増える、 (2) lint カテゴリの中で並列化の利得が小さい (vet / fmt はいずれも軽量)、 という点で overengineering。 lint / build / test の 3 分類に留め、 lint カテゴリは内部で複数 operation を順次実行する方を採用した。
- **classifier が `go list ./...` を呼んで module 内 package を enumerate する**: build tag / vendor / replace 等を toolchain 公式の解釈で扱える正確さがある。 一方、 (1) classifier に Go toolchain 起動の重い処理が入る、 (2) module が壊れている (compile error) ときに `go list` 自体が失敗して classifier が落ちる、 という不利益がある。 当面は find + `^package main` の grep という軽量手段で enumerate し、 build tag 等の解釈が必要になった時点で `go list` ベースに切り替える。
- **build / test の trigger を変更ファイルの所属 package のみに限定する (依存波及を classifier が解析しない)**: 検査範囲が狭まり 1 PR の検査時間が短くなる。 一方、 依存先 package への波及 (例: `internal/foo` 変更が `cmd/server` の build / test に影響) を classifier が見逃すと PR では緑、 main 後に壊れるという事故が起きる。 依存波及範囲を読み取る手段 (graph 解析) を持たない以上、 安全側に倒して module 内全 main package / 全 test package を再検査する方を採用した。
- **main package を module root 直下に置く (`<module>/main.go`)**: 1 アプリしか持たない module では階層が浅く済む。 一方、 (1) module 内に main package が増えた瞬間に再配置が必要になり過去 import path が壊れる、 (2) 配布可否を表現する suffix を main package のディレクトリで運ばせる本 ADR の規約とも整合しない、 (3) Go community 慣習 (`cmd/<name>/`) と外れ AI / 人間ともに main package の所在を毎回探す必要がある、 という不利益がある。 `<module>/cmd/<app_dir>/main.go` に固定して、 1 アプリでも複数アプリでも同じ階層で main package が見つかる形を採用した。
- **配布可否を `<app_dir>` 命名ではなく別の場所 (専用ディレクトリ `<module>/dist/`、 build manifest、 frontmatter コメント等) で表現する**: ファイル名の suffix を増やさずに済むが、 (1) 配布可否情報が main package の物理位置と分離するため AI / 人間がディレクトリツリーから一目で判別できなくなる、 (2) 別ファイル (manifest 等) を維持する手間が増え main package 追加時に同期漏れが起きる、 という不利益がある。 `<app_dir>` 自体に `_tool` suffix を付けて配布可否を物理位置に埋める方を採用した。
- **`_tool` 以外の suffix (`_dist` / `_bin` / `_release` 等) を採用する、 もしくは prefix で表現する (`tool_<name>`)**: 命名規約として等価だが、 (1) `<app_dir>` 名はバイナリ名と衝突しないように suffix で接尾語化する方が name 部分の再利用がしやすい (prefix にするとアプリケーション名から prefix を除く処理が必要)、 (2) `_tool` は 「外部 user に配布する CLI ツール」 を直感的に示し、 web server 等が主流になっても tool 概念で包括できる。 prefix / 別 suffix も機能としては成立するが、 接尾辞 `_tool` を採用した。
- **配布可否のマーカー (`_tool` suffix) を全 main package で必須化し、 内部用も `_internal_tool` 等の suffix を付ける**: 配布可否を suffix 不在 / 存在の二値で表現できる利点はあるが、 内部用の方が main package として多数派になりがちで、 多数派が長い suffix を背負うのは命名コストが高い。 配布可能 = `_tool` suffix 付き、 配布対象でない (default) = suffix なし、 という非対称命名で短い側を default に当てた。
- **配布対象 library module を main package 不在で判別する (suffix を使わない)**: ディレクトリ命名に制約を入れない自由度はあるが、 (1) library module に開発支援用 main package (検査用バイナリ、 demo CLI 等) を後から足したくなった瞬間に判別ロジックが破綻する、 (2) library 配布対象と tool 配布対象を直交に表現する手段が失われる、 という不利益がある。 module 名 suffix で配布性を表現し、 main package の有無とは独立に評価する方を採用した。
- **配布対象 library module の marker を suffix `_lib` のみに固定 (素の `lib` を許容しない)**: 命名の統一感は得られるが、 library が 1 つしか無い repo で `flame_lib/` のような冗長な命名を強要する。 module ディレクトリ名としての意味性 (= 「library を置く場所」) と短い `lib` 表記を許容したいケースを切り捨てない方を採用した。 2 形式 (`lib` / `<name>_lib`) を許容しても判別正規表現は `^(lib|.*_lib)$` の 1 行で済むため判別コストは増えない。
- **配布対象 library module の marker を internal な opt-in file (`<module>/.distributable` 等) で表す**: ディレクトリ名の自由度は最大化されるが、 (1) ファイルツリー一覧から「どれが配布対象 library か」 を一目で読めなくなり既存の `_tool` suffix 規約 (= ファイルツリーから配布性を読む) と一貫性が崩れる、 (2) marker file の追加忘れで release 経路から取りこぼされても気付きにくい、 という不利益がある。 module 名 suffix で表現する方を採用した。

### 公開 struct の最小化方針

- **公開 struct を field exported のまま expose し、 caller に struct リテラルでの初期化を許容する**: 構築コストが小さく、 簡単な data transfer であれば十分。 一方、 (1) 任意 field の追加や default 値変更が caller 側 struct リテラルを壊す可能性があり backward incompatible になりやすい、 (2) 必須 / 任意の区別が型情報から読めず caller が godoc / source を読みに行く必要がある、 (3) AI 開発 harness ([FLM_ENG_0001](../engineering/FLM_ENG_0001__claude_code.md)) で AI が struct を構築する際 caller が default 値を網羅する手間が発生する、 という不利益がある。 構築 API を constructor + functional options に固定する方を採用した。
- **公開 struct を維持しつつ field を全て unexported にする (zap-style: `Logger` exported / fields unexported)**: 型を caller 側で signature に書ける利点があり、 revive の `unexported-return` とも整合する。 一方、 (1) AI / 人間が「このモジュールは struct を expose している」とコード形態から誤読しやすい、 (2) 中間 Config 値を caller 側 variable に保持する誘惑が残り、 副次的に「Config を再利用する API」 が後付けで膨らみがち、 という不利益がある。 wrapper / library 層では struct 自体を完全 private 化し、 caller には method 連鎖か facade のみを expose する方を採用した。
- **builder pattern (`b.WithX(v).Build()` chain) を採用する**: 流れるような記述が可能だが、 (1) builder 自体が公開 struct となり同じ問題を再生産する、 (2) Go community で functional options pattern の方が広く採用されており参照可能なコード例が多い、 という不利益がある。 functional options pattern を採用した。
- **本規約を ADR ではなくモジュールごとの個別判断に委ねる**: モジュールごとに最適な API 設計を選べる柔軟性が得られる。 一方、 module 横断で API 形態が分散し、 (1) AI / 人間が新しい module の公開 API を読むときに毎回構築規約を把握する必要、 (2) module 間で共通する utility 設計が阻害される、 という不利益がある。 APP カテゴリの ADR で flame 全体の単一情報源として固定する方を採用した。
- **revive の `unexported-return` rule を有効に保つ**: exported function が unexported 型を返すパターンは Go community 一般では code smell と扱われ、 lint で抑止する慣習がある。 一方、 §公開 struct の最小化 で採用した「struct 自体を private 化し、 exported constructor から型推論経由で値を返す」 設計と直接競合する。 同方針を ADR で正面採用したため、 lint 側の rule を外して整合させた。
- **exhaustruct を flame 配下の struct のみに限定 (`include` filter で flame import path に絞る)**: cobra.Command 等の外部 多 field struct を partial literal で書ける利点はあるが、 (1) AI / 人間が「ここは外部 struct だから手抜きできる」と認識を持ちやすく、 同種の bypass を別 struct でも書きがち、 (2) 「全 struct で field 網羅を強制する」 という単一規律から逸脱して規約説明が複雑になる、 という不利益がある。 全 struct を対象にし、 外部多 field struct は flame 内に中央 constructor を 1 つだけ作って zero value を全 field 明示で初期化する方を採用した。
- **`new(T)` を許容して exhaustruct 違反の零値生成を許す**: 短く書ける利点があるが、 (1) struct literal を介さずに零値を確保する経路が残ると exhaustruct の「全 field 明示」 規律を AI / 人間がいつでも回避できる、 (2) `new(T)` は struct literal に比べて zero value の field 一覧が読み手に見えず、 後続の field 代入が漏れた場合に無音で zero が混入する、 という不利益がある。 forbidigo で `new` identifier を全面禁止し、 構築は struct literal または constructor 関数のみとする方を採用した。
- **wrapcheck の `ignore-package-globs` に flame package を登録し、 flame 間 cross-package call の wrap を不要にする**: caller 側 boilerplate (`return ex.Wrap(other.Func())`) を削減できる利点があるが、 (1) flame 配下 package の関数が外部 package 由来 error を返した場合、 caller 側で wrap を要求しないため検出不能になり ex.Wrap の実装漏れが顕在化しなくなる、 (2) 「外部 package boundary では必ず wrap」 という単一規律が崩れる、 という不利益がある。 ignore-package-globs は使わず、 cross-package boundary では明示的に ex.Wrap を呼ぶ規約に統一した。 ex.Wrap は idempotent (既に stacktrace を持つ error は再 wrap せず元の error を返す) のため、 flame 内で 2 段以上 ex.Wrap が連なっても stacktrace が増えない。

過去に採用していた決定として以下の経緯がある。

- 当初は wrapper package が定義する struct 型を例外なく package private にする方針 (DTO のみ例外) で運用していた。 service-level test (FLM_APP_0009) で `FakeIO` のような test double を導入した際、 test caller 側が verification method (`fake.VerifyStdout(t, expected)` 等) を named variable から呼ぶ必要があり、 test double を private 化すると test 側 boilerplate (型推論で受けても method 呼び出しを型有り変数経由にできない、 helper 戻り値型を anonymous interface にする等) が膨れることが見えたため、 test double に限った carve-out を追加した。 production 用途の wrapper / builder 型は引き続き全て private を維持する。

### premature publishing 方針

- **「将来公開するかも」 という見込みで先回り publish する**: 後から public 化する re-export 作業のコストが省ける利点はあるが、 (1) 実際に cross-package で利用されない名前まで API contract に乗ると godoc が膨れ、 AI / 人間が「この package の利用方法」 を読み取りにくくなる、 (2) public 名は backward-compatibility 制約を背負うため後続の変更コストが増える、 という不利益がある。 必要になった時点で private → public に昇格する形を採用した。
- **§premature publishing を完全に静的 lint で検出する**: 「同じ package 内からしか参照されていない exported 名」 を検出する linter があれば自動化できる。 一方、 cross-package import の有無を見る lint は「現時点で外から呼ばれていない」 を検出できても、 「将来呼ばれる予定がある (したがって public のままで良い)」 という意図と区別できない。 具体的に、 lib package が将来 cross-package で使われることを想定した name は static には判定できないため、 完全な静的化は不可能。 AI レビュー (test-coverage-reviewer / general-practices-reviewer) で補完する形を採用した。
- **interface の method 集合は全て public で揃える**: caller が interface 経由で method を呼ぶ Go の慣習では method を public にする方が自然。 一方、 caller package が直接 method を呼ぶ必要が無く、 interface を「満たす型を制限する contract」 として使う場合 (sealed interface) は method を private にすることで type 階層の閉鎖性が compile-time に保証される。 IO interface のような「caller は constructor 戻り値を Run に渡すだけで method を直接呼ばない」 利用パターンは sealed interface に該当するため、 method を private にする方を採用した。

### 戻り値型方針

- **戻り値型を ADR で固定しない**: 関数ごとに interface / concrete の最適選択を著者に任せる柔軟性が得られる。 一方、 (1) module 横断で API 形態が分散し caller 側の利用パターンが揃わない、 (2) §公開 struct の最小化 で採用した「constructor は private 型 (concrete) を返し caller は型推論で受ける」 規約と整合させる単一情報源が無い、 という不利益がある。 「accept interfaces, return concrete types」 を明文化する方を採用した。
- **interface 戻り値を全面許容する**: caller が「この値は IO を満たす」 ことだけに依存したい場合の表現が直感的になる。 一方、 (1) caller が concrete 型固有の method を後から使いたくなった時に再 type assertion が必要、 (2) compile-time に「特定 concrete が特定 interface を満たす」 ことを assert したい場面で `var _ Iface = (*Concrete)(nil)` 1 行で済む形が壊れる、 (3) Go community の慣習から外れる、 という不利益がある。 「accept interfaces, return concrete types」 を default 規約とし、 標準ライブラリ慣習由来の例外 / sealed interface 例外のみ carve-out する方を採用した。
- **sealed interface 戻り値も含めて全面 concrete 戻り値を強制する**: §戻り値型 の単一規律を維持できる利点はある。 一方、 §公開 struct の最小化 で wrapper / library package の struct を全 private にする方針と組み合わせると、 cross-package で値を受け渡す手段が消滅する (= caller package で型を書けないため variable に保持できず、 当該 wrapper 値を引き回せない)。 sealed interface (package private method を持つ interface) を例外として許容することで、 当該パターンの cross-package 受け渡しを可能にし、 implements 側 (concrete 型) は依然 private に保てる方を採用した。 caller が値の引き回しのみ行い method を直接呼ばない (sealed) ため、 「caller の柔軟性を奪う」 という ireturn 一般の懸念は本例外パターンに該当しない。

### package 命名方針

- **Go community 慣習 (all-lowercase, no underscores) に従う**: Effective Go / standard library の慣習と揃い、 godoc / 既存 Go コード基盤との見た目の一貫性が得られる。 一方、 CLI subcommand 名がハイフン区切り (例: `flow-document`) の場合、 対応する package 名 (= 連結した `flowdocument`) との対応関係が機械的に追跡できなくなる (= subcommand 名 → package 名の変換規則が「ハイフン除去」 と単一にならず、 復元時に元のハイフン位置が分からない)。 snake_case を採用してハイフンを `_` に 1:1 置換する方を採用した。 caller は subcommand 名 (`flow-document`) と package 名 (`flow_document`) を機械的に往復できる。
- **camelCase / PascalCase を採用する**: package 名のコンパクトさが得られる利点はあるが、 Go では package 名は import path 末尾としても使われる識別子であり、 大文字始まり / 大文字混在は Effective Go の lowercase 推奨から大きく逸脱する。 snake_case の方が標準推奨からの逸脱量が小さく、 underscores の使用は Effective Go でも「必要な場合は許容」 と読める範囲に収まる。
- **kebab-case を採用する**: subcommand 名 (ハイフン区切り) と完全一致できる利点があるが、 そもそも Go の identifier 規則上 package 名にハイフンは使用できない (= 言語仕様の制約)。 採用不可。
- **Go 予約語と衝突する subcommand 名 (例: `go`) を許容する**: subcommand 名と package 名を完全一致できる利点があるが、 Go 予約語 (= keyword) は package 名として使用できない (= 言語仕様の制約)。 衝突する subcommand 名は別の package 名 (`golang/` 等) にリネームする carve-out で運用する。

### error 表現と stacktrace 方針

- **標準 `errors.New` / `fmt.Errorf` のみで error を扱う**: 追加依存ゼロで起動が速い。 一方、 stacktrace を持たないため AI / 人間が CLI 失敗時の原因 frame を辿れず、 wrapcheck で外部 error を強制 wrap する場合の wrapper 群 (Wrap / Wrapf 等) も自前で書く必要がある。 stacktrace 付き wrapper を採用する方を採用した。
- **3rd party の error library (`github.com/pkg/errors` / `github.com/rotisserie/eris` / `github.com/cockroachdb/errors` 等) を採用する**: 機能が揃った実装を再利用でき初期実装コストが低い。 一方、 (1) error wrapper は flame の基盤 utility であり外部依存に左右されると flame 全体の信頼性に影響する、 (2) `pkg/errors` は archive 済、 `cockroachdb/errors` は sentry-go / gogo/protobuf 等の重量級 transitive dep を引き込む、 `eris` は active だが依存先が止まれば flame も止まる、 (3) 必要な機能 (stacktrace 捕捉 / wrap / unwrap chain / fmt.Formatter) は標準ライブラリ (`runtime.Callers` / `errors` / `fmt`) で完結する範囲に収まる、 という事情がある。 wrapper を標準ライブラリのみで自前実装し、 3rd party 依存をゼロにする方を採用した。
- **wrapper を経由せず標準 `errors` / `fmt.Errorf` を直接使う**: 追加 utility 不要で起動が速い。 一方、 標準 `errors` は stacktrace を持たず CLI 失敗時の原因 frame を辿れず、 wrapcheck 規約 (外部 error の wrap 強制) を満たすための wrap utility も自前で書く必要がある。 ex package を 1 つ用意して module 横断の stacktrace 付与経路を統一する方を採用した。

### go.mod directives 方針

- **`replace` を許容して fork や local 開発時の差し込みを認める**: 一時的な dependency 差し込みが容易になり、 upstream 修正待ちの間に local 改修で先行検証できる利点がある。 一方、 (1) AI 開発 harness ([FLM_ENG_0001](../engineering/FLM_ENG_0001__claude_code.md)) で AI agent が依存解決の問題を `replace` で workaround する誘惑が高く、 配布対象 library module ([FLM_FEA_0004](../feature/FLM_FEA_0004__release_policy.md)) の release artifact で同じ解決が再現できず利用側で壊れる、 (2) CI と local の解決経路が食い違って "local では緑だが CI で fail" 事象が起きる、 (3) `replace` の出所 (local path / fork URL) を caller が個別に追う必要が生じ依存の透明性が下がる、 という不利益がある。 `replace` を全面禁止する方を採用した。 fork が必要な依存は import path を fork 側 (= 別 module path) に切り替えるか、 upstream に PR を送って解決する。
- **`replace` を local path 形式のみ禁止し、 remote module 形式 (`replace foo => bar v0.0.1`) は許容する**: fork module への切り替えを `require` 書き換え無しで運用できる利点がある。 一方、 (1) caller (利用側 module) が `require` 行だけを読んでも実際の解決先が分からず、 解決の出所が `replace` まで降りないと読み取れない非対称が残る、 (2) AI agent が「local じゃないから OK」 と remote 形式の `replace` を多用し始めると同じ workaround 文化を再生産する、 という不利益がある。 local / remote の両形式を一律禁止する方を採用した。
- **`replace` 禁止を ADR で固定せず PR レビューで都度判断する**: その場の事情に応じた柔軟性が得られる。 一方、 (1) 静的にチェックできるルールを静的チェックで担保する flame の方針 ([FLM_GEN_0004](../general/FLM_GEN_0004__static_check.md)) と整合しない、 (2) AI ターン内で `replace` が混入しても fail せず後段の人間レビューでしか弾けない、 という不利益がある。 `gomoddirectives` linter による静的検査に乗せ、 `.golangci.yaml` の設定差分として明示する方を採用した。

過去に採用していた決定として以下の経緯がある。

- 当初は lint カテゴリを Go 公式 toolchain (gofmt + go vet) と `go mod tidy -diff` のみで構成し、 community 標準の meta linter (golangci-lint) は導入せず将来要件として保留していた。 AI 開発 harness ([FLM_ENG_0001](../engineering/FLM_ENG_0001__claude_code.md)) として lint を「機械的に解ける問題を AI ターン内で検出 / fix する」サイクルに乗せるには公式 toolchain だけでは検出力が薄かったため、 golangci-lint を採用し errcheck / staticcheck / errorlint / gosec / gocritic / revive 等の community 標準 lint ルール群を一括で取り込む構成に改訂した。 設定はリポジトリルートの `.golangci.yaml` に置き、 default ruleset を使わず採用 linter / 設定を明示列挙する形 (バージョン非依存) を採用した。
- 当初は main package の配置を module 内の任意位置 (典型的には module root 直下 `<module>/main.go`) に許容し、 命名規約も置かなかった。 1 module = 1 アプリの前提で運用していたが、 (1) 1 module に main package を複数置きたいケース (CLI と検査用ツール、 配布対象と非配布対象等) が見えてきたとき配置と命名のばらつきが避けられず、 (2) 配布対象かどうかをディレクトリツリーから読み取る手段が無かったため、 `<module>/cmd/<app_dir>/main.go` に固定する規約と、 配布可能アプリへの `_tool` suffix 規約を導入した。 既存の `cli/main.go` は `cli/cmd/flame_tool/main.go` に再配置する。
- 当初は context.Context の伝搬規約を ADR で固定せず、 main / cobra wrapper / cross-package boundary の関数 signature に ctx を取らない構成だった。 IO / IPC / 外部依存呼び出しを含む関数で cancel / deadline を扱う手段が無く、 service-level test (FLM_APP_0009) でも test framework の per-test ctx を駆動経路に渡せなかったため、 主要な関数境界で ctx を第一引数に取る規約 (Go community 慣習に従う) を §context 伝搬 として明文化した。 副作用を持たない pure utility (ex package の error wrapper 等) は carve-out として ctx を取らない。 cobra wrapper では root command が ctx を受け取り `SetContext` で cobra tree 全体に伝搬する形に統一した。
- 当初は §戻り値型 で sealed interface 例外を持たず、 「accept interfaces, return concrete types」 の例外は標準ライブラリ慣習由来 (`error` / `io.Reader` 等) の動的選択戻り値のみを carve-out していた。 §公開 struct の最小化 で wrapper / library package の struct を全 private にする方針と組み合わせると、 wrapper が組み立てた値を cross-package で受け渡す手段が `var _ Iface = (*Concrete)(nil)` の compile-time assertion では足りない (caller 側で型を書けないため variable に保持できず引き回せない) ことが [FLM_APP_0008](FLM_APP_0008__cli.md) §subcommand package の階層 の実装を通じて顕在化した。 sealed interface (package private method を持ち外部 package が実装できない interface) を戻り値とするパターンは「caller が値の引き回しのみ行い method を直接呼ばない」 ため ireturn の一般懸念 (caller の柔軟性を奪う) に該当しないと判断し、 §戻り値型 の carve-out として sealed interface 例外を追加した。 同経緯で ireturn linter は当該パターンを広範に検出するため、 [FLM_APP_0010](FLM_APP_0010__code_comment.md) §lint 「本決定と本質的に衝突する lint rule は無効化する」 経路に沿って ireturn 自体を `.golangci.yaml` から外した。
- 当初は §error 表現と stacktrace で error wrapper package を CLI 実装 module 自身の `<module>/lib/ex/` に置く規約だった (配布対象 library module 自身が wrapper を expose する場合のみ `<module>/ex/` への carve-out を持っていた)。 配布対象 library module を release 経路に追加 ([FLM_FEA_0004](../feature/FLM_FEA_0004__release_policy.md)) するに伴い `lib` module を新設して wrapper を library module の export root 直下 (`<lib_module>/ex/`) に集約し、 cli module は自身に wrapper を持たず lib module の `ex` package を import する形に変えた ([FLM_APP_0008](FLM_APP_0008__cli.md) §main package と CLI フレームワークの分離 の clix wrapper と並行する経緯)。 これにより error wrapper の依存箇所が flame 全体で 1 module (lib) に集約され、 非 lib module 側で wrapper を重複保持しない構造になった。
