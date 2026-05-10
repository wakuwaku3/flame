# 配布対象の release policy: GitHub Release を介した tool / library 系統の自動 release

## 背景

- ソースコードを GitHub に置くプロジェクトでは、 GitHub の tag 紐付け release 機構と release asset (任意ファイルの添付) が利用できる
- 1 tag = 1 release。 tag 名はリポジトリ内で一意で、 Go module の subdirectory module convention では `<module>/v<semver>` 形式が広く使われる
- セマンティックバージョニング (semver) は `MAJOR.MINOR.PATCH` の 3 数値で構成され、 互換性の有無を版番号で表現する慣習として広く普及している
- GitHub Actions は `push` イベントで branch / tag を契機にワークフローを起動できる ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md))
- CI 検査は PR の `pull_request` を契機とし、 main マージ前の最終ゲートとして機能する ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md))
- Go の toolchain は `GOOS` / `GOARCH` 環境変数による cross-compile を標準で提供し、 シングルバイナリ生成も標準で完結する ([FLM_APP_0007](../application/FLM_APP_0007__go.md))
- 1 リポジトリに複数の配布対象 main package が並存する可能性がある ([FLM_APP_0007](../application/FLM_APP_0007__go.md))。 release の系譜 (前回版 → 今回版) は配布対象ごとに独立に進む
- 配布対象アプリケーション (`<app_dir>` が `_tool` suffix 付き main package) は `<module>/cmd/<app_dir>/` に配置する ([FLM_APP_0007](../application/FLM_APP_0007__go.md))。 各 module は独立した `go.mod` を持つ
- 配布対象 library module は module ディレクトリ名が `lib` または `_lib` suffix を持つ ([FLM_APP_0007](../application/FLM_APP_0007__go.md))
- CLI 実装は cobra Command tree を走査して公開 surface (subcommand / flag) を機械的に列挙する隠し subcommand (`__spec`) を持つ ([FLM_APP_0008](../application/FLM_APP_0008__cli.md))
- PR には変更ファイルパスから `module/<name>` 形式の label を自動付与する規約がある ([FLM_ENG_0004](../engineering/FLM_ENG_0004__github_label.md))
- curl は HTTP / HTTPS リソースをシェルへパイプ実行できる CLI として広く普及しており、 `curl ... | bash` 形式の install スクリプト配布は OSS で広く採用されている
- GitHub の検索 API は label 名と日時範囲で merged PR を絞り込める (`gh pr list --state merged --label <name> --search "merged:>=<datetime>"`)
- GitHub release は `published_at` フィールドを持ち、 release 作成時刻が API から取得できる
- コンテンツ種別ごとに「作成 skill / lint / build / test / ADR ルール検査 skill」の 5 項目を整備する規約がある ([FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md))

## 決定

release 対象を以下の 2 系統に分類して扱う。 各系統で起動契機・版番号方針・リリースノート規約・CI 分離は共通とし、 spec 抽出方式・tag 命名・asset 形式・配布プラットフォーム・インストール経路で系統間の差異を持つ。

- **tool 配布**: [FLM_APP_0007](../application/FLM_APP_0007__go.md) §配置 で定める `<app_dir>` = `<app_name>_tool` の main package。 OS / アーキテクチャ別バイナリと CLI 公開 surface (cobra subcommand / flag) JSON spec を release asset として配布
- **library 配布**: [FLM_APP_0007](../application/FLM_APP_0007__go.md) §配置 で定める module ディレクトリ名が `lib` または `_lib` suffix を持つ Go module。 git tag のみで配布されるためバイナリ asset は持たず、 Go 公開 API surface (exported 識別子集合) JSON spec のみを release asset として配布。 spec 抽出は配布対象 module 自身ではなく外部 tooling が AST 走査して行う (詳細は §版番号の決定経路)

以下のサブセクションでは tool / library で共通する規約を主軸とし、 系統で分かれる項目は **tool** / **library** ラベルでそれぞれ明記する。

### リリース起動契機

- `main` ブランチへの push を契機に、 配布対象 (tool / library 両系統) の release を自動生成する
- 系統ごとに独立した実体層 workflow ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md) §実体層命名) が enumerate と release を担う (tool / library は別 workflow として並列起動)
- 1 起動で当該系統内の全配布対象を処理し、 配布対象 (アプリケーション or library module) ごとに独立に release を作る (並列展開)
- 配布対象ごとに、 前回 release tag → 今回 commit の間に当該 module ディレクトリ配下のファイル変更が 1 件も無い場合は release を作らない (tag push と `gh release create` を skip する)。 module 内に変更が無いまま PATCH 採番だけが進むと、 release notes が `module/<name>` label PR 0 件で生成される (= release が中身を持たない) 状態になり、 リリースノート規約 ([§リリースノート](#リリースノート)) と矛盾するため。 初版 release (前回 tag が存在しない配布対象) はこの判定を経由せず常に作成する

### 版番号

- 版番号は semver の `MAJOR.MINOR.PATCH` で管理する (tool / library 共通)
- 配布対象ごとに独立に採番する (tool ならアプリケーションごと、 library なら module ごと)
- 初版は `1.0.0` とする (tool / library 共通)
- 既存版がある場合の bump は前回版との差分で決定する。 差分の入力 (= 公開 surface) は系統で異なる:
  - **tool**: CLI 公開 surface ([FLM_APP_0008](../application/FLM_APP_0008__cli.md)) の差分
  - **library**: Go 公開 API surface (exported 識別子集合と signature の string 表現) の差分。 抽出経路は §版番号の決定経路 を参照
- bump kind の対応付けは両系統共通:
  - **MAJOR**: 公開 surface が破壊的に変化したとき
  - **MINOR**: 公開 surface に追加があり、 破壊的変化が無いとき
  - **PATCH**: 上記いずれにも該当しない変更 (公開 surface の単純削除を含む)
- 公開 surface の各要素 (tool: subcommand / flag、 library: 関数 / 型 / メソッド / 変数 / 定数 / フィールド / interface method) の rename・型変更・required 化は破壊的変化、 純粋追加は機能追加、 単純削除は破壊的変化とみなさない (PATCH に分類する) — 両系統共通
- **library 系統限定: MAJOR の自動 release を行わない**: Go の subdirectory module convention では import path が `<module_path>/v<MAJOR>` (MAJOR ≥ 2) suffix を要求するため、 自動 release は `v1.x.x` の範囲内に限定する。 自動 bump 判定が MAJOR を返した場合 release ワークフローは fail する。 v2 以降は import path 変更を伴う手動 module migration として ADR 範囲外で扱う

### 版番号の決定経路

- 各 release では、 配布対象の公開 surface を表す JSON spec をビルド成果物として生成し、 release asset として添付する (tool / library 共通)
- 次回の bump 判定は前回 release の spec asset と今回ビルドの spec を比較して行う (tool / library 共通)
- spec を生成する責務は系統で異なる:
  - **tool**: 配布対象アプリケーション自身が emit する ([FLM_APP_0008](../application/FLM_APP_0008__cli.md) §公開 surface 抽出経路 で各 CLI に課される一般規約)。 release ワークフロー側は外部から CLI 表面を解釈する手段を別途持たず、 配布対象自身の隠し subcommand を呼び出す
  - **library**: 外部 tooling が AST 走査で抽出する。 配布対象 library module 側に spec emission の実装は持たせない (CLI 形態でない library module に main package を強制しないため)。 抽出は当該 module 配下の non-internal package について exported 識別子集合と signature の string 表現を JSON 構造として標準出力に emit する。 抽出実装は 3rd party 依存を採用せず、 Go 標準ライブラリの `go/parser` / `go/ast` / `go/token` 等で完結させる

### tag

- tag 名は系統で異なる:
  - **tool**: `<app_name>/v<MAJOR>.<MINOR>.<PATCH>`
  - **library**: `<module_path>/v<MAJOR>.<MINOR>.<PATCH>` (`<module_path>` = repo root から module ディレクトリまでの相対パス)。 これは Go subdirectory module convention に一致するため、 Go module proxy が `go get <module_path>@v<semver>` でそのまま取得可能
- tag は release 作成時に自動生成する。 ローカル / PR では作らない (tool / library 共通)

### release asset

release に添付する asset は系統で異なる。

- **tool**: 次の asset を添付する
  - OS / アーキテクチャの組合せごとに 1 つのバイナリアーカイブ
  - CLI 公開 surface を表す JSON spec (次回 bump 判定の入力)
  - 添付した archive 群に対する checksum
- **library**: 次の asset を添付する
  - Go 公開 API surface を表す JSON spec (次回 bump 判定の入力)
  - バイナリアーカイブと checksum は持たない (Go module は git tag のみで配布されるため)
- アーカイブの命名は OS / アーキテクチャ / バージョン / アプリ名を install スクリプトと release ワークフローが機械的に判別可能な命名規則に従う (tool 系限定)

### 配布プラットフォーム

- **tool 系**: 配布対象 OS / アーキテクチャの集合は、 主要なデスクトップ / サーバ OS の amd64 / arm64 を最低限カバーする。 個別の OS / arch 組合せは release ワークフロー (matrix) 側で持ち、 ADR では集合の policy のみを規定する
- **library 系**: Go module proxy 経由配布のため OS / arch 非依存。 配布プラットフォーム policy は持たない

### リリースノート

リリースノート規約は tool / library 両系統共通。 「当該 module」 とは tool 系では配布対象アプリケーションの module、 library 系では配布対象 library module を指す。

- release notes には当該リリース系譜 (前回 release → 今回 release) の間に main にマージされた、 当該 module の `module/<name>` label が付いた PR ([FLM_ENG_0004](../engineering/FLM_ENG_0004__github_label.md)) を一覧化する
- 系譜の境界は GitHub の commit 比較機能で前回 tag → 今回 commit の commit 集合として確定する。 各 commit に紐づく PR の解決は immediate consistent な経路に限定する
- PR の解決は squash-merge で GitHub が default で commit subject 末尾に付与する `(#<number>)` を抽出し、 当該番号で PR detail を直接取得する。 PR ↔ commit の reverse-lookup endpoint や search index 経由の検索は eventual consistent で merge 直後に取りこぼすため採らない
- 番号が抽出できない commit (merge-commit / rebase-merge / 直接 push された commit 等) は Changes 一覧から除外する
- 初版 release は前回 tag が無いため Changes 一覧を空欄もしくは省略の体裁にする
- 一覧には PR 番号・タイトル・著者を含める
- release notes には install 手順 (curl pipe one-liner) を併記する (tool 系限定)
- release ワークフローは tag push / release 作成等の side effect を suppress する dry-run モードを持つ。 同じ実体層 workflow を手動実行エントリポイントから dry-run 入力で起動できるようにし、 production code path をそのまま動作確認できる経路を提供する

### インストール

**tool 系限定**。 library 系は Go module proxy 経由 `go get <module_path>@v<semver>` で利用側が直接取得するため、 install スクリプト / 配布元への認証経路 / shell completion 配置は持たない (これらは配布対象アプリケーションが提供する CLI に固有の関心事のため)。

- 配布対象アプリケーションごとに、 GitHub release から指定版 (省略時は最新版) を取得してローカルにインストールする shell スクリプトを当該 module 内に配置する
- 当該 install スクリプトは pipe 実行 (`curl ... | bash` 等) で起動でき、 引数で任意のバージョン指定が可能でなければならない
- 配布元リポジトリの可視性に応じて install スクリプトは認証経路 (Personal Access Token / `gh auth` 等) をサポートし、 private リポジトリ配布でも release asset を取得できる
- インストール先はユーザ書き込み可能な default を持ち、 環境変数で上書き可能とする
- install スクリプトは冪等に動作する (target 版が既に install されていれば再 download を行わない)
- shell completion ([FLM_APP_0008](../application/FLM_APP_0008__cli.md)) の物理配置までを install スクリプトの責務とする。 install スクリプトは利用者の shell を検出し、 当該 shell の completion 配置慣習に従ってファイルを生成 / 配置する。 配置を抑止したい場合の opt-out 経路 (環境変数等) を提供する

### CI 経路との分離

- リリース処理は CI 検査 ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md)) とは独立した実体層 workflow として実装し、 CI 検査の workflow / トリガーには混入させない

### 5 項目の整備状況

[FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md) で定める 5 項目について以下を整備する。

| 項目 | 整備方針 |
| --- | --- |
| 作成 skill | 省略 (release 処理は GitHub Actions skill ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md)) と shell スクリプト skill ([FLM_APP_0002](../application/FLM_APP_0002__shell_script.md)) で完結する) |
| lint | 各種別の既存 lint (Go / Shell / GitHub Actions / YAML / Markdown) を継承 |
| build | release 処理自体が cross-compile build を含むため別 build は省略 |
| test | 省略 (実 release は main マージ後にのみ走るため、 PR では release 経路は実行しない) |
| ADR ルール検査 skill | 省略 (各種別の lint に委譲) |

## 影響

- 配布対象 module 内に変更がある main マージで release が走り、 公開 surface 不変の変更 (内部実装のみの変更等) であっても PATCH bump にフォールスルーするため、 当該 module 内の変更を含む main マージごとに少なくとも PATCH は必ず上がる
- 一方、 配布対象 module ディレクトリ配下に 1 件もファイル変更が無い main マージ (他 module や repo root の運用ファイルのみが変わる commit 等) では当該配布対象の release は生成されず、 採番もスキップされる ([§リリース起動契機](#リリース起動契機))。 結果として release tag は配布対象 module の実変更と 1:1 で対応する
- 1 リポジトリ内の全配布対象アプリケーションが並列に release され、 各々が独立した tag / release を持つ。 GitHub release 一覧には配布対象ごとに `<app_name>/v...` 形式 (tool 系) もしくは `<module_path>/v...` 形式 (library 系) の tag が並ぶ
- 配布対象側は版を上げる責務を持たない (workflow が semver bump を自動で決める)。 逆に開発者が手動で `v2.0.0` を打ちたいケースは本決定の範囲外
- tool 系で公開 surface 判定が CLI 表面 (cobra の subcommand / flag) に固定されるため、 内部実装変更 (パフォーマンス改善、 リファクタ等) は MAJOR / MINOR の対象にならず、 すべて PATCH に分類される
- subcommand / flag の単純削除を MAJOR に上げないため、 削除のみのリリースは PATCH となる。 削除版の利用者は意図せず壊れる可能性があるが、 削除を含む rename は本 ADR では検出されず別の名前の追加として扱われる (削除 + 追加で MINOR に上がる)
- 次回 bump 判定は前回 release の spec asset 取得を伴うため、 release asset への spec JSON 添付が次回判定の前提になる。 当該 asset を release から削除した場合、 次回ビルドは前回 spec を取得できず判定が壊れる
- tag 名前空間が配布対象ごとに `<app_name>/...` (tool 系) ないし `<module_path>/...` (library 系) で分割されるため、 同一リポジトリ内の配布対象間で tag が衝突しない
- Windows binary は `.exe` suffix が Go toolchain 仕様で固定され、 アーカイブ展開後のバイナリ名にもそれが残る。 install スクリプトはプラットフォームによってバイナリ名末尾の suffix を考慮して PATH 上に置く必要がある
- install スクリプトを `curl ... | bash` で配布する慣習を採用するため、 任意の利用者がリポジトリを clone せずに CLI を入手可能となる。 半面、 install スクリプトの内容は配布前に十分静的検査 (shellcheck 等) を通している必要がある
- 配布元リポジトリが private のときは install スクリプトの取得 (raw 配信) と release asset の download の両方で GitHub への認証を要する。 install スクリプトは `GITHUB_TOKEN` env もしくは `gh auth token` のいずれかで token を解決し、 release は GitHub API の asset id 経由 (`Accept: application/octet-stream`) で取得する。 認証情報が無い場合は明示的に fail する
- インストール先 default が PATH に含まれない場合、 install 後にユーザへ PATH 追加を案内する必要がある
- shell completion は install スクリプトが利用者 shell を検出して物理配置するため、 「install しただけで completion が効く」 体験を提供する。 半面、 install スクリプトに OS / shell 検出ロジック・配置先慣習 (XDG Base Directory / shell ごとの site-functions など) ・opt-out 経路を抱える必要がある
- 利用者 shell の検出は親シェル (`$SHELL`) を参照するため、 curl pipe 経由でも親シェルを正しく取得できる。 検出不能 / 自動配置対象外 (現状 windows の PowerShell) の場合は install スクリプトが案内 note のみを出す fallback を持つ
- 配置先は default で XDG Base Directory に従うが、 環境変数で override / opt-out 可能とする。 具体パス・環境変数名は install スクリプト本体に持たせ、 ADR では抽象 policy のみを保持する
- release ワークフローは `pull_request` 経路と分離されるため、 PR 段階では release 処理が走らない。 release 経路の挙動を PR で確認する手段として workflow の `workflow_dispatch` 起動を使う
- リリースノートに載る PR 一覧は `module/<name>` label が付いた merged PR に限定されるため、 fork PR ([FLM_ENG_0004](../engineering/FLM_ENG_0004__github_label.md) で label が付かない) や label 付与前の merged PR は取りこぼされる
- リリースノート生成は前回 tag → 今回 commit の commit 集合を起点に絞り込むため、 同一 module で複数 release が短時間に連続したとき各 release のノートは互いに排他になる (PR が複数 release に重複して載らない)
- 比較機能が返す commits 数には実装上の上限がある (compare REST では 250 件) ため、 release 系譜が当該上限を超えた場合は超過分の PR が release notes に載らない。 release 自体は成功扱いとなり、 release notes が部分的になった事実は CI ログの警告と release 完了後の運用者確認で拾う責務分担とする (通常 release 間隔では発生しない)
- 同じ compare REST が返す files 数にも実装上の上限がある (300 件)。 [§リリース起動契機](#リリース起動契機) の「module 内変更が無い main マージは release を作らない」 判定はこの files 集合を path prefix で絞り込むため、 truncation で当該 module の変更ファイルが返却分 300 件の外に並ぶと「実変更があるのに skip される」 取りこぼしを起こす。 これを避けるため、 commits / files いずれかが上限に張り付いた兆候を検出した場合は安全側 (= release を進める) にフォールバックし、 警告を CI ログに残す (通常 release 間隔では発生しない)
- PR 解決を commit message 内の番号 (`(#<number>)`) 抽出に固定するため、 merge-commit / rebase-merge を使う運用や直接 push される commit は release notes Changes セクションに載らない (skip 件数は CI ログに警告として残す)
- 同じ実体層 workflow に dry-run 入力を持たせるため、 release tag を作らずに production code path (build / spec emission / 版番号 算出 / release notes 生成) を任意の feature branch 上で再現できる。 release 経路の挙動を main マージ前に検証する手段が常時提供され、 検証用に別系統の workflow ファイルを保守する必要がない
- tool / library で実体層 workflow が分離するため、 一方の系統で 0 件 (例: library module が無いリポジトリ) でも片系統だけ release が走る。 各系統の workflow は配布対象 0 件のときに固定 success を返す (CI 検査 / release の責務分離方針 ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md)) と整合)
- library 系の bump 判定は Go 公開 API surface (exported 識別子集合と signature の string 表現) の差分で行う。 §版番号 の「公開 surface 各要素の純粋追加 = MINOR、 既存要素の rename・型変更 = MAJOR」 の判定軸と spec の identifier 1 件を 1:1 対応させるため、 struct / interface 宣言は型本体 (kind=type で signature は bare `struct` / `interface` に固定) と exported field / interface method の atomic identifier に分解して emit する。 これにより interface への method 追加・struct への exported field 追加は new spec 側で identifier の追加 (= added > 0) として MINOR に分類され、 既存 method の signature 変更や field 型変更は同 identifier の signature 差 (= shape_changed) として MAJOR に分類される。 unexported field / 内部実装変更は spec に現れず PATCH に落ちる。 v1.x.x の範囲内では MAJOR 判定が release ワークフローの fail を意味し、 v2 移行は import path 変更を伴う手動 module migration として ADR 範囲外で扱う
- library 系の release asset は Go 公開 API spec JSON の単一ファイルのみ。 利用側は git tag (`<module_path>/v<semver>`) を介して `go get <module_path>@v<semver>` で取得するため、 release asset 自体は次回 bump 判定の入力としてのみ消費される
- library 系は Go module proxy 経由配布のため、 install スクリプト / 認証経路 / shell completion 配置等の tool 系固有の運用関心事は持たない (これらは tool 系限定の §インストール 規約に閉じる)
- 1 module が library 配布対象 (例: `<name>_lib/`) と tool 配布対象 (例: `<name>_lib/cmd/<app>_tool/`) の両方に該当する場合、 同 module を input にした 2 系統の release が並列に走る。 tag 名前空間が tool 系 (`<app_name>/...`) と library 系 (`<module_path>/...`) で分離するため衝突しない
- 「決定」 で抽象 policy として残した具体実装 (アーカイブ命名フォーマット文字列、 配布対象 OS / arch の個別組合せ、 install スクリプトの具体配置パス・default 環境変数名・default install 先、 release / 検査の workflow ファイル名、 リリースノート markdown のセクション見出し) は release ワークフロー定義 / install スクリプト本体 / 関連 module 内ドキュメントに分散して保持される。 これらは仕様変更時に各実装側で更新する
- 本 ADR の規約は依存側プロジェクトへ伝播する (本 ADR は FEA カテゴリかつ downstream に位置付けられるため)

## 評価

代替案として以下を検討した。

### リリース / インストール方針

- **release を自動化せず開発者が手動で tag を打つ**: semver の判断を人間が下せる利点はあるが、 (1) main マージのたびに開発者が tag 操作を要求され忘れやすく、 (2) AI 主体の開発フローでは AI が release 判断・tag 操作までは担いづらい、 (3) 公開 surface の差分は機械的に判別可能なため自動化と相性が良い。 main マージ自動 release を採用した。
- **tool 系の公開 surface 判定を CLI 表面以外 (公開 Go API、 設定ファイルスキーマ等) も含めて行う**: より厳密な互換性追跡が可能だが、 (1) tool 配布の本質は CLI バイナリの「実行時に利用者が触る surface」 であり、 cobra の subcommand / flag 集合がそれを完全に表す、 (2) Go API は library 配布対象として独立した release 系統で扱う方が責務が明快、 という事情がある。 tool 系は CLI 表面に限定し、 library 系は別系統として Go API surface を判定軸とする方を採用した。
- **library 配布対象の bump 判定を tool と統合し、 1 系統で扱う**: 系統が 1 つに整理される利点があるが、 (1) tool は CLI 表面、 library は Go API と判定軸が異なり、 1 系統で両軸を扱うと spec 構造 / diff ロジック / asset 形式が混在して肥大化する、 (2) tool は OS / arch matrix のクロスコンパイルを伴い library は伴わない等、 build 経路の責務が直交する、 という事情がある。 系統を tool / library に分割し、 各々で独立した実体層 workflow を持つ方を採用した。
- **library 配布対象の MAJOR 自動 bump を許容する (`/v2`、 `/v3` ... の import path migration を release ワークフローが担う)**: 利用者から見れば MAJOR が自動で進む利点はあるが、 (1) Go の subdirectory module convention で MAJOR ≥ 2 は import path 末尾に `/v<MAJOR>` の追加を要求し、 module 配下の go.mod / 全 import 文 / 依存側プロジェクトでの import 経路まで一斉書き換えが必要、 (2) 当該書き換えは AST 解析で機械的に可能だが、 release ワークフロー内で他 PR / 他 module を巻き込む変更を生むのは責務範囲外、 (3) v1.x.x の範囲では MAJOR 自動 bump が事故の温床になりやすい (内部実装変更を MAJOR と誤判定するケース等)、 という事情がある。 library 系は MAJOR 判定を release ワークフローの fail として扱い、 v2 移行は手動 module migration として ADR 範囲外で扱う方を採用した。
- **library 系の公開 API spec を library module 自身が emit する (= tool 系と同型化、 配布対象 module に main package を強制)**: 抽出責務が当該 module 側に集約される利点はあるが、 (1) library 配布対象に「spec emission 用 main package」 の存在を強制すると本来の library 形態 (main 不在の純粋な package 集合) を破壊する、 (2) 全 library module で同じ AST 走査ロジックが重複、 (3) cross-compile / spec emission バイナリのビルドコストが累積、 という不利益がある。 spec emission 責務を外部 tooling に集中させ、 library module 側は spec emission を持たない方を採用した (§版番号の決定経路)。
- **library 系の公開 API spec 生成に `gorelease` / `apidiff` (`golang.org/x/exp` 配下) を採用する**: 公式相当の精度が得られる。 一方、 (1) `x/exp` は実験的位置付けで API 変動リスクがある、 (2) tool 系の CLI spec diff と library 系の Go API spec diff を同型のロジック (flat key-value 比較) で扱いたい、 という事情がある。 自前の AST 走査で flat key-value spec を emit する方を採用した。
- **subcommand / flag の単純削除を MAJOR に上げる**: 利用者から見れば削除も互換性破壊だが、 (1) 内部利用者向け CLI が中心で削除のたびに MAJOR を上げると版番号が肥大化する、 (2) ユーザの想定には「不要機能の削除は普通の改善」 という見方もあり、 厳密な MAJOR 化は AI 主体の小刻みな改善サイクルと相性が悪い。 単純削除は PATCH に分類する方を採用した。 削除を含む rename (削除 + 別名追加) も MINOR にとどまる挙動になるが、 これは検出側の構造的限界として受け入れる。
- **公開 surface の比較を ソースコード解析 (cobra の利用箇所を AST 走査) で行う**: build を伴わずに spec 取得が可能だが、 (1) cobra は実行時に flag 登録が完了する設計のため AST 解析だけでは登録漏れを検出できないケースがある、 (2) 解析実装が大きく、 cobra のバージョンアップで AST 構造が変わると追従コストがかかる。 配布対象アプリケーション自身に spec emission 隠し subcommand を持たせ、 build 結果を実行することで spec を取り出す方を採用した。
- **公開 surface の spec を別ファイル (`cli-spec.json` 等) としてリポジトリに commit する**: 前回版を git 履歴から取得できる利点があるが、 (1) spec の commit 漏れが発生すると判定が壊れる、 (2) コードと spec の二重メンテになる。 spec をビルド成果物として release asset に添付し、 git 管理から外す方を採用した。
- **tag 命名を `v<semver>` (アプリ名 prefix なし) にする**: 1 リポジトリ 1 アプリの場合は最短だが、 同一リポジトリに複数アプリが並存する場合に tag が衝突する。 アプリ名 prefix を必須化する方を採用した。
- **tag 命名を `<app_name>-v<semver>` (ハイフン区切り) にする**: 命名としては等価だが、 Go module の subdirectory module convention (`<dir>/v<semver>`) と整合しない。 Go module 慣習に従う方を採用した。
- **install を Homebrew / scoop / apt 等のパッケージマネージャ経由で配布する**: ユーザの install 体験は最も洗練されるが、 (1) パッケージマネージャ各々の formula / manifest 維持コストが高い、 (2) リポジトリ単独で完結する curl pipe 配布の方が AI 主体の開発フローと相性が良い (AI がパッケージマネージャ向け manifest を逐次更新する手間を省ける)。 curl pipe 配布を主経路として採用し、 パッケージマネージャ対応は別 ADR で扱う。
- **install スクリプトをリポジトリルートに置く**: アプリケーションが 1 つの間は単純だが、 複数アプリに拡張するとリポジトリルートに `install-<app>.sh` が並んで配置の責務が散る。 アプリケーションごとに当該 module 内に置く方を採用した。
- **install 先 default を `/usr/local/bin` にする**: PATH に最初から含まれることが多い利点があるが、 sudo 必須となり curl pipe install のシンプルさが崩れる。 ユーザ書き込み可能な path (例: `$HOME/.local/bin`) を default にし、 環境変数で上書きできる形を採用した。
- **release ワークフローを CI 検査ワークフローに同居させる**: ファイル数を抑えられるが、 (1) CI 検査は `pull_request` 起点 / release は `push` 起点と起動契機が異なる、 (2) 検査と release の責務が混ざることで CI 検査 workflow の意味が曖昧になる、 (3) [FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md) の「実体層は個別 verb 単位」 規約に反する。 release は target ごとに独立した実体層 workflow として独立させた (ファイル名は配布対象種別を表す target を含める形で [FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md) §実体層命名と [FLM_APP_0007](../application/FLM_APP_0007__go.md) の `_tool` / `_lib` セマンティクスに揃える)。
- **tool / library の release ワークフローを 1 ファイルに統合する**: workflow ファイル数を抑えられるが、 (1) tool は OS / arch matrix の cross-compile を伴い library は伴わないなど、 jobs の構造 / matrix shape が系統で根本的に異なる、 (2) [FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md) §実体層命名 が target を file 名で表現する規約のため target が 2 軸あれば file も 2 つに分けるのが整合的、 (3) 1 系統で 0 件 (例: library が無いリポジトリ) のとき、 統合 workflow だと無関係な job 構造をスキップする条件分岐が肥大化する、 という不利益がある。 系統ごとに workflow を独立させ、 上位 fan-out 実体層が両者を再利用可能呼出 ([FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md) §実体層同士の合成) で並列起動する方を採用した。
- **fan-out 実体層を持たず、 tool / library それぞれに独立したトリガー層を持つ**: 各系統のトリガーが直接対応する実体層を呼ぶため fan-out 階層を 1 つ削れる。 一方、 [FLM_ENG_0003](../engineering/FLM_ENG_0003__github_actions.md) §トリガー層 で定める「discriminator は当該 event 内の挙動を一意に区別する snake_case」 規約のもと、 push 系トリガーでは discriminator が branch / tag filter と整合する必要があり、 同一 event (push to main) を契機とする 2 トリガーは discriminator が衝突する。 fan-out を実体層 (1 ファイル 1 ジョブ規約の対象外) に置き、 トリガー層は 1 ファイル 1 ジョブのまま fan-out 実体層を呼ぶ方を採用した。
- **install スクリプトを各 module 内ではなくリポジトリルートに集約し、 `<app_name>` を引数で渡す形にする**: 1 ファイルで多 app を捌けるが、 (1) アプリごとの install は各アプリの利用者にとって独立体験で、 共通スクリプトに集約すると認証・OS 検出・PATH 案内のロジックが多 app を考慮した条件分岐で膨らむ、 (2) 配布の curl pipe URL がアプリごとに異なる方が利用者の認知負荷が低い。 module 内配置を採用した。

### リリースノート方針

- **GitHub の自動生成 release notes (`gh release create --generate-notes`) をそのまま使う**: GitHub が前回 tag からの merged PR を自動列挙する利点があるが、 (1) 列挙対象は時系列で全 PR となり、 multi-app monorepo で「他 app の PR が混ざる」問題を解けない、 (2) `.github/release.yml` でカテゴリ分けはできるが label による絞り込みはできない。 自前で label PR を列挙する方を採用した。
- **release notes に `git log` ベースの commit 一覧を載せる**: PR を経由しない直接 commit (squash-merge 以外) も拾える利点があるが、 (1) main への変更を PR 経由に統一する運用では commit 単位での粒度は冗長、 (2) commit メッセージは PR タイトルより形式が統一されておらず読みづらい。 PR 単位を採用した。
- **release notes 起点を「前回 tag の commit 日時」 にする**: tag を打った commit の `committer date` を起点にする案があるが、 squash-merge を介した main の commit 時刻と PR の merged 時刻はずれることがあり datetime ベースの絞り込みは PR との整合性が脆い。 commit ベース (commit 比較機能) で系譜を確定させ、 各 commit に紐づく PR を REST で解決する方を採用した。
- **リリースノート生成を GitHub の `.github/release.yml` で代替する**: カテゴリ分け機構は得られるが、 module ごとの絞り込みができないため multi-app monorepo 構成と整合しない。 `.github/release.yml` は採用しない。

### ADR スコープ / 経緯

- **release / install 規約を CLI ADR ([FLM_APP_0008](../application/FLM_APP_0008__cli.md)) に同居させ続ける**: 旧運用ではこれを採用していた。 (1) リリース機構は cobra の Command tree から spec を取り出すため CLI 実装と密結合、 (2) 1 ADR で release / install / CLI を扱う方が cross-reference が減る、 という利点があった。 一方で release / 配布の決定 (tag 命名・asset・配布プラットフォーム・GitHub の release 機構固有の運用) は CLI 実装と独立に進化する性質があり、 また label を消費するリリースノート規約が新規追加されるに伴って CLI 実装の関心事から離れた。 GitHub Release / 配布周りの規約を独立 ADR に切り出した。
- **GitHub Release 系 ADR と GitHub Label ADR を 1 本に統合する**: release / label が現状はセットで運用されるため統合する案もあったが、 (1) label は release 以外の用途 (人間によるトリアージ等) でも将来使われる可能性があり、 (2) 「label を付与する側 (CI)」 と「label を消費する側 (release)」 を別 ADR にした方が責務が明快になる。 別 ADR とした。
- **本 ADR (release policy) を internal な release 経路実装と同居させる**: 1 ADR で release 全体を扱うと cross-reference が減る利点があるが、 (1) downstream プロジェクトに伝播させたい一般 policy と、 flame 自身の release 経路実装 (固有の workflow ファイル名・CLI subcommand 名・install スクリプト path・自身の version 採番との連動詳細) は責務が直交する、 (2) [FLM_GEN_0007](../general/FLM_GEN_0007__resource_classification.md) の internal / downstream 分類のもと、 1 ADR が両性質を抱えると当該 ADR をどちらに分類するかが決まらない、 という不利益がある。 downstream な一般 release policy を本 ADR に切り出し、 internal な release 経路実装は別 ADR (FLI_FEA_0001) に閉じる方を採用した。

過去に採用していた決定として以下の経緯がある。

- 当初は配布対象アプリケーションの release / install 規約を [FLM_APP_0008](../application/FLM_APP_0008__cli.md) (CLI 実装の基本ルール) 内に同居させていた。 release notes に label PR 列挙を載せる規約 ([FLM_ENG_0004](../engineering/FLM_ENG_0004__github_label.md) を消費する) を追加するにあたり、 GitHub Release 固有の決定群を独立 ADR (FLI_FEA_0001) に切り出し、 [FLM_APP_0008](../application/FLM_APP_0008__cli.md) からは当該セクションを削除した。 [FLM_APP_0008](../application/FLM_APP_0008__cli.md) は引き続き spec emission 機構など CLI 実装の本体規約を持ち、 release 系 ADR は当該 spec を入力として消費する側として cross-reference する。
- 当初は release notes の PR 列挙を GitHub Search API (`gh pr list --label <name> --search "merged:>=<published_at>"`) で実装していた。 main マージ直後の自動 release で merge から `gh pr list` 実行まで時間が経過しても search index への反映が間に合わず、 PR が release notes から取りこぼされるケースが顕在化した。 immediate consistent な REST endpoint (compare で commit 集合を確定 → 各 commit の `/commits/{sha}/pulls` で PR を解決 → label で client 側絞り込み) に切替えた。
- 上記の REST 切替え版でも、 PR ↔ commit の reverse-lookup endpoint (`/commits/{sha}/pulls`) が同様に eventual consistent な index に依存することが判明した (merge 直後の reverse lookup に PR が反映されておらず Changes セクションが空になるケースが発生)。 commit message に GitHub が default で付与する `(#<number>)` を抽出して `/pulls/<number>` (immediate consistent な PR detail endpoint) で解決する経路に再切替えし、 `/commits/{sha}/pulls` への依存を撤去した。
- 同時に、 release 経路を main マージ前に検証する手段として、 当初は別系統の workflow + 専用 wrapper script を新設する形を採っていた。 これは GitHub Actions の制約 (新設 workflow は default branch に存在しないと workflow_dispatch で起動できない) により feature branch 上での検証経路として実用的でなかった上に、 production の deploy workflow とは別 code path を検証するため「production と同じ経路を踏んでいるか」 が保証できない問題があった。 既存の deploy 実体層 workflow 自身に dry-run 入力を持たせ、 同じ workflow が tag push / release 作成だけを skip する形に統合した。 これにより workflow file は既に main にあるため feature branch から workflow_dispatch で起動でき、 検証する code path が production と完全一致する。
- 当初は「main マージごとに必ず PATCH が上がる」 という挙動を §影響 で明示しており、 module 内に 1 件も変更が無い main マージでも全配布対象の release が走っていた。 結果として release notes が `module/<name>` label PR 0 件で「変更無し」 と表示される release が連発し、 リリースノート規約 ([§リリースノート](#リリースノート)) と矛盾する状態になった。 [§リリース起動契機](#リリース起動契機) に「前回 release tag → 今回 commit で当該 module ディレクトリ配下のファイル変更が 0 件なら release を生成しない」 旨を追加し、 release tag が配布対象 module の実変更と 1:1 で対応するようにした。 判定は path ベース (gh compare API の `.files[].filename` を当該 module dir の prefix で絞り込み) とする。 label PR 件数で判定する案も検討したが、 fork PR / label 付与前 merge / merge-commit などで label が付かない経路で誤検知 (= 実変更があるのに skip) を起こしやすい一方、 path ベースは GitHub の compare API の生 file 集合に直結するためこれらの取りこぼしが無い。
- 当初は配布対象を tool 系 (`_tool` suffix 付き main package) のみとし、 公開 surface 判定も CLI 表面 (cobra subcommand / flag) に固定して「ライブラリ用途は前提にしていない」 と評価していた。 [FLM_APP_0007](../application/FLM_APP_0007__go.md) §配置 の module 命名規約に library 配布対象 marker (`lib` / `_lib` suffix) を追加し、 配布対象 library module を release 対象に加える際、 Go 公開 API surface を判定軸とする library 系を tool 系と並置する形に拡張した。 spec 抽出責務は library module 側ではなく外部 tooling に集中させ、 実体層 workflow も tool 系 / library 系の 2 系統に分割した。 既存 tool 系の起動契機・tag 命名・asset 形式・リリースノート規約は不変に保ち、 library 系を独立した経路として並列起動する。 library 系の MAJOR 自動 bump は Go の subdirectory module convention (`/v<MAJOR>` import path 規則) と相性が悪いため自動 release は v1.x.x の範囲内に限定し、 v2 以降は手動 module migration として ADR 範囲外で扱う。
- 当初は release policy と internal な release 経路実装 (固有の workflow ファイル名・CLI subcommand 名・install スクリプト path・自身の version 採番との連動詳細・dry-run 検証実装の固有事情等) を 1 ADR (FLI_FEA_0001) に同居させていた。 [FLM_GEN_0007](../general/FLM_GEN_0007__resource_classification.md) の internal / downstream 分類のもと、 release policy 部分は依存側プロジェクトへ伝播させたい downstream 性質を持ち、 internal な実装詳細とは責務が直交することが顕在化した。 release policy 部分を本 ADR ([FLM_FEA_0004](FLM_FEA_0004__release_policy.md)) として独立させ、 FLI_FEA_0001 は flame 自身の release 経路実装 (固有の workflow / CLI subcommand / install スクリプト等) に縮小した。
