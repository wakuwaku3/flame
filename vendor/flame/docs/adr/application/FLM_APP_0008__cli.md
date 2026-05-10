# CLI 実装の基本ルール

## 背景

- flame は CLI ツールを Go で実装する方針である ([FLM_APP_0007](FLM_APP_0007__go.md))
- flame は AI エージェントとの協働開発を前提として設計する ([FLM_GEN_0002](../general/FLM_GEN_0002__flame.md))
- Go の CLI 実装には複数の論点がある: CLI フレームワークの選定 (標準ライブラリ `flag` / community 製ライブラリ (`spf13/cobra`、 `urfave/cli`、 `alecthomas/kong` 等) / 自作)、 main package 内の構造、 配布対象 / 内部利用バイナリの判別、 引数解析の必要可否など
- `spf13/cobra` (以下 cobra) は Apache License 2.0 で配布される Go の CLI フレームワークであり、 kubectl / hugo / gh / docker / helm 等の主要 CLI で採用されている。 subcommand ツリー、 自動 help 生成、 flag parsing (POSIX 準拠の long / short)、 shell completion、 version flag 等を組み込みで提供し、 `github.com/spf13/cobra` として Go module 経由で配布される
- main package の配置 (`<module>/cmd/<name>/main.go` / `<name>_tool` suffix による配布対象判別) と Go ファイル / module manifest の規約は [FLM_APP_0007](FLM_APP_0007__go.md) で定義されている
- flame の static check (lint / build / test) は checker 規約 ([FLM_FEA_0001](../feature/FLM_FEA_0001__checker.md)) に従って組み立てる
- flame ではコンテンツ種別ごとに「作成 skill / lint / build / test / ADR ルール検査 skill」の 5 項目を整備する規約がある ([FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md))
- cobra の `Command` は Subcommand と Flag を子要素として持つツリー構造であり、 root から再帰的に走査することで CLI が公開する surface (subcommand と flag の集合) を機械的に列挙できる
- 配布対象アプリケーションの GitHub Release 運用 (リリース起動契機、 版番号、 tag、 asset、 リリースノート、 install 等) は [FLM_FEA_0004](../feature/FLM_FEA_0004__release_policy.md) で定義されている

## 決定

flame では Go で CLI を実装する際の規約を本 ADR で集約する。 CLI 実装に関する個別の決定をここに並べ、 新しい論点が現れた場合は本 ADR に追記する。 main package の配置 / 配布対象判別 (`_tool` suffix) など Go コード全般の規約は [FLM_APP_0007](FLM_APP_0007__go.md) で扱い、 release / install 規約は [FLM_FEA_0004](../feature/FLM_FEA_0004__release_policy.md) で扱う。 本 ADR ではそれらを再記述しない。

### CLI フレームワーク

flame では Go で CLI を実装する場合、 CLI フレームワークに **cobra** を採用する。 module 横断で単一の CLI フレームワークに固定し、 各 module で個別選定はしない。

### CLI フレームワークの適用範囲

- 引数 / subcommand を持つ CLI 形態の Go バイナリ (配布対象 / 内部利用を問わず) は cobra で実装する
- 引数を取らない単純なバッチ (引数解析が不要なもの) は cobra を必須としない (標準ライブラリで足りる)

### main package と CLI フレームワークの分離

- main package は CLI フレームワークの root command 生成と起動のみを担い、 subcommand / flag の定義は別 package に分離する
- CLI フレームワークへの直接依存は wrapper package 1 つに集約する。 wrapper は配布対象 library module ([FLM_APP_0007](FLM_APP_0007__go.md) §配置 で定める module 名 `lib` / `_lib` suffix) の export root 直下 (`<lib_module>/clix/`) に配置する (= 当該 module 自体が flame の library layer を担うため、 import path が `github.com/.../<lib_module>/clix` の形になる)。 CLI 実装 module は自身に wrapper を持たず、 当該 library module の wrapper を import して利用する
- root / subcommand 実装 package は CLI フレームワーク (cobra) を直接 import せず、 wrapper のみを介して command 構造を組み立てる
- 内部 endpoint (§公開 surface と内部 endpoint の区別 で定義する `__` prefix の subcommand) は wrapper が「システムコマンド」として一括提供する。 wrapper は root command 生成時にシステムコマンドを自動登録し、 root / subcommand 実装 package は内部 endpoint を意識しない

### subcommand package の階層

- subcommand 実装を持つ Go package は CLI コマンド階層と物理ディレクトリ階層を 1:1 で対応させて配置する
- root command 用 package は `<module>/internal/root/` に置く
- 公開 subcommand `<cmd1> [<cmd2> [<cmd3>]]` の package は `<module>/internal/root/<cmd1>[/<cmd2>[/<cmd3>]]/` に置く
- 各 subcommand package は wrapper (§main package と CLI フレームワークの分離) の command 値を組み立てて返す関数を公開し、 親 command の package が当該関数を呼んで wrapper の API で AddCommand する。 直接の親 package 以外から子 package を import しない
- システムコマンド (内部 endpoint) は本階層規約の対象外で、 wrapper package が提供する

### 公開 surface と内部 endpoint の区別

- CLI には「公開 surface」 (利用者向けに提供する subcommand / flag) と「内部 endpoint」 (release / 自動化機構が消費する補助 endpoint) を区別する
- 内部 endpoint は subcommand 名のプレフィックスを `__` とし、 CLI フレームワークの仕組みで help 出力から除外する
- 公開 surface 抽出機構 (CLI spec の出力など) は内部 endpoint を surface とみなさず除外する

### 公開 surface 抽出経路

- 配布対象アプリケーション自身が cobra Command tree を走査して公開 surface (subcommand / flag) を JSON spec として標準出力に emit する隠し subcommand を提供する
- 当該 spec は release ワークフロー ([FLM_FEA_0004](../feature/FLM_FEA_0004__release_policy.md)) が版 bump 判定の入力として消費する

### shell completion

- shell completion は CLI フレームワーク (cobra) が標準で提供する completion subcommand を採用する。 各 CLI で個別に completion 実装を持たない

### 5 項目の整備状況

[FLM_GEN_0005](../general/FLM_GEN_0005__content_type.md) で定める 5 項目について以下を整備する。

| 項目 | 整備方針 |
| --- | --- |
| 作成 skill | 省略 (CLI 実装は Go ファイル作成 skill ([FLM_APP_0007](FLM_APP_0007__go.md)) と shell スクリプト作成 skill ([FLM_APP_0002](FLM_APP_0002__shell_script.md)) に委譲) |
| lint | Go / Shell / GitHub Actions / YAML / Markdown 各種別の既存 lint を継承 |
| build | Go の build に委譲 ([FLM_APP_0007](FLM_APP_0007__go.md)) |
| test | Go の test に委譲 ([FLM_APP_0007](FLM_APP_0007__go.md)) |
| ADR ルール検査 skill | 省略 (各種別の lint に委譲) |

## 影響

- CLI を持つ Go module の `go.mod` に `github.com/spf13/cobra` が依存として加わる
- 各 CLI で help / version / completion 等の体裁が cobra default に揃う
- cobra の Command 構造体の組み立て方のうち subcommand / flag 命名や登録順、 root command 生成 helper の有無は CLI ごとの判断に委ねるため、 module 横断で書き方が完全には揃わない余地が残る (subcommand package の物理配置は §subcommand package の階層 で固定する)
- cobra の breaking change 発生時は依存 module 全てで追従が必要になる
- cobra の transitive dependency (`spf13/pflag`、 `inconshreveable/mousetrap` (Windows のみ) 等) も `go.sum` に取り込まれる
- cobra で表現できない CLI 形態 (標準入出力主体の filter、 REPL 等) が必要になった場合は本 ADR の対象外として個別判断する
- main package と CLI フレームワークの分離方針により、 main package 自体は薄く保たれ、 subcommand / flag を担う package を独立に lint / test できる
- subcommand package の物理配置が CLI コマンド階層と 1:1 で対応するため、 AI / 人間は CLI 上の `flame <cmd1> <cmd2>` から実装ファイルの位置 (`<module>/internal/root/<cmd1>/<cmd2>/`) を機械的に特定できる
- subcommand を追加するたびに `<module>/internal/root/` 配下にディレクトリを 1 段切る必要がある (フラットな単一 package で済まないため、 小さい CLI でもディレクトリ数が階層数に応じて増える)
- 内部 endpoint は階層規約の対象外で wrapper package が「システムコマンド」として一括提供するため、 公開 subcommand と内部 endpoint で配置のルールが分岐する。 利用者向け CLI surface (`<module>/internal/root/<cmd>/.../`) はディレクトリツリーから機械的に把握できる一方、 内部 endpoint は wrapper の実装を読む必要がある
- flame には CLI フレームワーク wrapper package (`<lib_module>/clix/`) を 1 つ持つ。 cobra への直接依存と subcommand 構造の組み立て API、 内部 endpoint (システムコマンド) の実装が本 package に集約され、 CLI 実装 module は当該 wrapper を import して利用する
- root / subcommand 実装 package が cobra を直接 import しないため、 cobra の breaking change / API 更新の追従コストは wrapper package 内に閉じる。 wrapper の API 改訂は wrapper 利用者 (root / 全 subcommand 実装) を全数追従させる必要がある
- wrapper を経由する分、 cobra の細かい機能 (annotation / PersistentPreRun 等) を subcommand 実装側から直接利用できなくなる。 必要な機能は wrapper の API として明示的に export する形で取り込む
- 本 ADR は APP カテゴリのため、 依存側プロジェクトが Go で CLI を実装する際にも本規約が伝播する (subcommand 階層・wrapper 集約・システムコマンドの取り扱いを含む)
- gosec の `G304` (Potential file inclusion via variable) は CLI ツールの本質と相容れない。 配布対象 CLI / 公開 subcommand (例: `flame ci release spec lib <module-path>`) は利用者から filesystem path を引数で受け取って読む API を持つため、 path 引数の非リテラル化を全面禁止する G304 は false positive 生成器となる。 path 入力の sanitize は CLI 引数仕様で別途設計されるため、 lint 層で一律に検出する価値が無い。 [FLM_GEN_0006](../general/FLM_GEN_0006__no_lint_suppression.md) §lint config ファイルにはコメントを書かない / 理由は ADR §影響 に書く 経路に沿って `.golangci.yaml` 側で `gosec.excludes` に `G304` を登録する。 G304 は CLI / library / 取り込み側 repo を含む全 module に共通する false positive のため、 vendor SoT の shared lint config (`vendor/flame/.golangci.yaml` → install 後 root の `.golangci.yaml`) で登録する
- gosec の `G703` (Path traversal via taint analysis) も同じ事情で CLI 側に false positive を生む。 `flame install` (FLM_FEA_0003) の vendor sync / install copy 経路は、 `flame.yaml` 由来の path / `flame.lock` の install path / 副ファイル overlay path を `os.WriteFile` / `os.ReadFile` で読み書きするのが endpoint 責務であり、 taint 元が「外部入力」 と判定される全てが構造的に CLI の本質責務に含まれる。 G304 と異なり G703 は taint chain 解析を行うため、 同じ path 値経路でも検出条件が異なる場合があるが flame の CLI / install 経路では両方が同様に false positive 化する。 ただし G703 は library (`lib/`) module 側では発火条件が無い (= 当該 module は filesystem path を引数で受け取る endpoint を持たない) ため、 `gosec.excludes: G703` の影響範囲は実質的に CLI module に閉じる。 検査契約を CLI module の現実だけに合わせる方が ADR §評価 で論じる「設定値の意図と実効範囲を一致させる」 観点に整合するため、 G703 は CLI 専用 lint config (`cli/.golangci.yaml`) のみで `gosec.excludes` に登録する (vendor SoT の shared config には登録しない)
- gosec の `G204` (Subprocess launched with variable) も同様に CLI に固有の false positive。 `flame install` は `flame.yaml.harness.version` を `git clone --branch <version>` の引数として渡す経路 (FLM_FEA_0003 §チャネル C) を持ち、 version 値は manifest 経由で利用側が指定する parameter であって shell 呼び出し用の任意文字列ではない。 同様の subprocess 起動経路 (`go test` / `golangci-lint` / `gh` 等の起動) は flame check / flame ci の各 subcommand に内在し、 G204 はこれらの全箇所で false positive を生み続ける。 当該経路は library module には存在しないため、 G204 は CLI 専用 lint config (`cli/.golangci.yaml`) のみで `gosec.excludes` に登録する
- gosec の `G122` (Filesystem operation in WalkDir without root-scoped APIs) は symlink TOCTOU 攻撃面を懸念する rule で、 `flame install` の vendor sync が temp dir に git clone した内容を repo 内へ rsync する経路 (FLM_FEA_0003 §チャネル C) で発火する。 当該 temp dir は git checkout 直後の閉じた tree で symlink TOCTOU 攻撃面が無く (= 攻撃者は temp dir に介入できない)、 `os.Root` 等の root-scoped API への切替えは flame の CLI 経路で攻撃面を縮減せず lint 充足のためだけのコード追加となる。 当該経路は library module には存在しないため、 G122 は CLI 専用 lint config (`cli/.golangci.yaml`) のみで `gosec.excludes` に登録する
- 上記 3 件 (G703 / G204 / G122) を CLI 専用 lint config に分離する都合上、 flame self には `cli/.golangci.yaml` を別途配置する (`vendor/flame/.golangci.yaml` の near-duplicate に CLI 固有 excludes を追加した形)。 golangci-lint v2 は extends / include 機構を持たず、 cwd 起点で最寄りの `.golangci.yaml` を 1 ファイルだけ読み込む仕様のため duplicate を許容する (= cli module で golangci-lint を起動した際に cwd = cli/、 `cli/.golangci.yaml` が選択される)。 vendor SoT 側の lint config 改訂時は CLI 側 config も同期して更新する責務を負う (= 単一情報源としては vendor SoT、 CLI 側はその near-clone)
- CLI 実装に関する新たな論点 (例: ログ出力規約、 終了コード規約、 設定ファイル読み込み規約等) が現れた場合は本 ADR に決定を追加する形で集約する
- 内部 endpoint は cobra の `Hidden = true` で help 出力から除外し、 release ワークフロー ([FLM_FEA_0004](../feature/FLM_FEA_0004__release_policy.md)) は `__` prefix の subcommand (例: `__spec`) を呼び出して CLI 公開 surface を抽出する
- 公開 surface 抽出機構は subcommand 名の `__` prefix を判定して内部 endpoint を除外する
- 配布対象アプリケーション自身が CLI surface 出力を担うため、 CLI module には spec emission の実装 (cobra Command tree 走査) を持つ必要がある

## 評価

代替案として以下を検討した。

### ADR のスコープ

- **CLI 実装に関する ADR を「CLI フレームワーク選定」に限定し、 cobra 採用 1 件のみを扱う**: ADR タイトル / スコープが狭く明快になる。 一方、 CLI 実装には framework 選定以外の論点 (main package 構造、 ログ規約、 終了コード規約等) も発生し、 それらの ADR を都度新設すると APP カテゴリ内に CLI 関連 ADR が散在する。 本 ADR を「CLI 実装の基本ルール」 として CLI 関連の個別決定を集約する器に位置付け、 個別決定をセクション単位で並べる方を採用した。
- **CLI フレームワークを ADR で固定せず、 各 module で自由選択する**: module ごとに最適なフレームワークを選べる柔軟性が得られる。 一方、 module 横断で CLI のコード形態が分散し、 (1) AI / 人間が新しい CLI を読むときに毎回フレームワーク差分を把握する必要、 (2) module 間で CLI 補助関数 (root command 生成 helper 等) の共有が阻害される、 という不利益がある。 APP カテゴリの ADR で flame 全体の単一情報源として固定する方を採用した。

### CLI フレームワークの選定

- **標準ライブラリ `flag` のみで実装する**: 依存ゼロで起動が速く、 transitive dependency 管理も不要。 一方、 (1) subcommand ツリーが標準で提供されず CLI 規模が大きくなると独自実装が必要、 (2) help 生成・shell completion・POSIX 準拠の short flag (`-vv` 連結等) を毎回手作業で組む必要がある、 (3) AI 開発 harness ([FLM_ENG_0001](../engineering/FLM_ENG_0001__claude_code.md)) の AI が CLI を生成する際、 community 標準の cobra パターンを学習で把握している方が出力品質が安定する、 という不利益がある。 cobra を採用して subcommand / help / completion を一括で得る方を採用した。
- **`urfave/cli` を採用する**: cobra と同等の機能を提供し API はやや簡潔。 一方、 採用 CLI の規模 / community での認知度で cobra (kubectl / hugo / gh / docker / helm 等で採用) が大きく勝り、 AI / 人間ともに参照可能なコード例が圧倒的に多い。 cobra を採用した。
- **`alecthomas/kong` を採用する**: struct tag ベースで宣言的に CLI を定義できる利点がある。 一方、 cobra と比較して採用例が少なく AI / 人間ともに参照可能なコード例の絶対数で劣る。 cobra を採用した。

### main package と CLI フレームワークの分離方針

- **CLI フレームワーク採用を含めて main package のテンプレート (project layout 含む) を ADR で固定する**: より細かい統一が得られるが、 main package 自体の内部構造を全て規約化すると CLI の機能要件で変動する余地が残らない。 main package は cobra root command 生成と起動のみに固定し、 main package を越えた subcommand 側 (`<module>/internal/root/...`) の階層を別途固定する形 (§subcommand package の階層方針) を採用した。

### subcommand package の階層方針

- **subcommand package の物理配置を各 CLI の判断に委ねる (フラット / 階層任意)**: 命名・粒度の自由度は最大化されるが、 (1) AI / 人間が CLI 上の `flame <cmd1> <cmd2>` から実装ファイルを探すたびに module 内を grep する必要がある、 (2) 1 module に CLI が増えたとき配置のばらつきで隣接 package 間の規約衝突 (例: 一方が `<root>/checkadr/`、 一方が `<root>/check/adr/`) が起きる、 (3) AI 開発 harness ([FLM_ENG_0001](../engineering/FLM_ENG_0001__claude_code.md)) で AI が新しい subcommand を生成するときに毎回配置先の判断を要する、 という不利益がある。 階層を CLI コマンド階層と 1:1 で対応させ、 物理配置を CLI surface から決定論的に導出する方を採用した。
- **subcommand package を `<module>/cmd/<app_dir>/` 配下に置く**: main package と subcommand package が同居して関連が物理的に近くなる利点はあるが、 (1) `<module>/cmd/<app_dir>/` は [FLM_APP_0007](FLM_APP_0007__go.md) で main package の固定配置先と定義されており main package 以外を混ぜると配布対象 / 内部利用の判別 (§ `_tool` suffix) が曖昧になる、 (2) main package を thin に保つ §main package と CLI フレームワークの分離 と整合しない、 (3) 1 module に CLI が複数あった場合 (`cmd/<app_a>_tool/check/adr/`、 `cmd/<app_b>_tool/check/adr/` 等) で同じ subcommand 階層が複数 main package で重複し共有が阻害される、 という不利益がある。 `<module>/internal/root/` 配下に統一する方を採用した。
- **内部 endpoint (`__` prefix subcommand) も階層規約に強制し、 サニタイズした命名 (例: `internal/root/__spec/` を `internal/root/spec/`) を強制する**: 公開 / 内部の配置ルールが揃って一貫性が出るが、 (1) サニタイズ命名 (prefix `_` の除去等) で物理ディレクトリ名と cobra subcommand 名が乖離し、 ディレクトリ→ subcommand 名の機械的逆引きが壊れる、 (2) 内部 endpoint は `__` prefix の通り公開 surface ではなく、 `<module>/internal/root/` ツリーは「公開 CLI 表面」を反映する役割を負っているため、 内部 endpoint を同階層に並べると surface 概念が濁る、 (3) 内部 endpoint は CLI フレームワーク wrapper の責務 (§CLI フレームワークの隠蔽方針) と整合的に「システムコマンド」として wrapper 側で提供する方が実装一箇所で済む、 という不利益がある。 内部 endpoint は階層規約の対象外で wrapper が一括提供する方を採用した。
- **package 間の import を経由せず subcommand 群を全て root package に直接定義する**: package 数を最小化でき初期コストが低い。 一方、 root package が CLI の全 subcommand 実装を抱えて lint / test 単位が肥大化し、 [FLM_APP_0007](FLM_APP_0007__go.md) の package 単位 lint / build / test 単位を活かせない。 1 subcommand = 1 package で物理階層を CLI 階層に揃える方を採用した。

### CLI フレームワークの隠蔽方針

- **root / subcommand 実装 package が cobra を直接 import する**: wrapper を介さない分 cobra の機能を制限なく使える。 一方、 (1) cobra への依存が CLI 全 package に分散し breaking change 時の追従コストが線形に増える、 (2) AI / 人間が CLI 実装 package を読む際 cobra API の知識が前提になる、 (3) 内部 endpoint (`__spec` 等のシステムコマンド) を root package で組み立てることになり root が cobra Command tree 走査ロジックを抱える、 という不利益がある。 cobra への依存を 1 wrapper package (`<lib_module>/clix/`) に集約し、 root / subcommand 実装は wrapper API のみを import する方を採用した。
- **wrapper を `<module>/internal/clix/` に置く (内部用に閉じる)**: 当該 module 内に閉じる利点はあるが、 (1) flame は依存側プロジェクトでも cobra ベース CLI を実装する場合に同じ wrapper を共有したくなる可能性が高く、 internal/ では別 module から import できない、 (2) wrapper 自体は flame に固有の判断ではなく cobra 利用パターンの集約であり library 性が高い、 という事情がある。 配布対象 library module の export root 直下 (`<lib_module>/clix/`) に置いて公開 import path を保つ方を採用した。
- **wrapper 名を `cobra` 等の framework 名そのままにする**: framework との対応が直感的だが、 (1) 将来 cobra 以外への切り替えが起きた際に名前と実装が乖離する、 (2) framework 名そのままだと wrapper が薄い再エクスポートに見え、 wrapper を介する意図 (cobra 隠蔽 + システムコマンド集約) が弱まる、 という不利益がある。 `clix` (CLI extension の略) という framework 中立な名前を採用した。
- **システムコマンド (内部 endpoint) を wrapper ではなく独立 package で提供する**: システムコマンド責務が wrapper から分離されて wrapper が薄くなる利点はあるが、 (1) システムコマンドは root command の cobra tree への参照が必要で wrapper が NewRoot 時に注入する形が最も自然、 (2) システムコマンド package を別途設けると root / subcommand 実装は wrapper と システムコマンド package の 2 つを意識する必要が出る、 という不利益がある。 wrapper が NewRoot 時にシステムコマンドを自動登録する方を採用した。

### 公開 surface と内部 endpoint の区別方針

- **release / 自動化向けの endpoint も公開 subcommand として露出する**: 隠し subcommand を導入する必要が無く CLI が単一構造で済む。 一方、 (1) 利用者向けの help にリリース機構固有の subcommand が表れて公開 surface が不明瞭になる、 (2) CLI 公開 surface の bump 判定 ([FLM_FEA_0004](../feature/FLM_FEA_0004__release_policy.md) が利用者向け surface の差分から版番号を決める想定) が release 機構自身の subcommand 変更で誤って trigger される、 という不利益がある。 内部 endpoint を `__` prefix と help 除外で公開 surface から分離する方を採用した。
- **release / 自動化向けの機能を別バイナリに切り出す**: 公開 CLI から内部 endpoint を完全に排除でき責務が明快になる。 一方、 (1) 内部 endpoint が公開 CLI の root command tree から spec を取り出す用途であるため別バイナリだと公開 CLI のメモリ内 cobra tree にアクセスできず別経路で再構築が必要、 (2) 配布対象バイナリが増えて release 機構自体が複雑化する、 という不利益がある。 公開 CLI 内に隠し subcommand として同居させる方を採用した。

過去に採用していた決定として以下の経緯がある。

- 当初は本 ADR のタイトルを「CLI 実装には cobra を採用する」 とし、 ADR スコープを cobra 採用 1 件に限定していた。 cobra 採用は CLI 実装に関する個別決定の 1 件にすぎず、 main package 構造 / 配布対象判別 / 引数解析の必要可否などを並列に扱う必要があったため、 タイトルを「CLI 実装の基本ルール」 に broadening し、 CLI 実装に関する個別決定を本 ADR 内のセクションで集約する形に変更した。 既存の cobra 採用決定そのものは「CLI フレームワーク」 セクションとして保持している。
- 配布対象アプリケーションの release / install 規約は当初本 ADR に統合していた (それ以前は FLM_ENG_0002 として別 ADR、 統合時に本 ADR に吸収)。 release notes に label PR 列挙を載せる規約を加える際に、 GitHub Release 固有の関心事を [FLM_FEA_0004](../feature/FLM_FEA_0004__release_policy.md) として独立 ADR に再分離した。 本 ADR は CLI 実装本体 (cobra 採用、 公開 surface / 内部 endpoint の分離、 spec emission 機構) のみを扱い、 release ワークフローや install スクリプトの規約は [FLM_FEA_0004](../feature/FLM_FEA_0004__release_policy.md) を参照する形に整理した。
- 当初は shell completion の物理配置責務を本 ADR に書いていたが、 当該責務は install スクリプトが担うため [FLM_FEA_0004](../feature/FLM_FEA_0004__release_policy.md) §インストール に移した。 本 ADR には「cobra completion subcommand を採用する」 という CLI 側の決定のみを残した。
- 当初は CLI 内の subcommand package の物理配置を各 CLI の判断に委ねる運用 (§main package と CLI フレームワークの分離 までで規約を止めた状態) としていた。 cli module で root command 用の package を `<module>/internal/cmdroot/` に置いて 1 ファイルに全 subcommand を定義する状態が続いており、 (1) AI / 人間が CLI 上の `flame <cmd1> <cmd2>` から実装位置を探すたびに module 内 grep を要する、 (2) subcommand を増やしたときの配置先がコードレビューで都度議論になる、 (3) AI 開発 harness で AI が新規 subcommand を生成するときに配置の判断点が増える、 という運用上の摩擦が顕在化した。 §subcommand package の階層 で `<module>/internal/root/<cmd>/.../` に物理配置を固定する規約を導入し、 cli module の `internal/cmdroot/` を `internal/root/` に再配置した。
- 当初は CLI フレームワーク (cobra) への依存を root / subcommand 実装 package が直接保持する構成で、 内部 endpoint (`__spec`) 用の utility package (`cli/internal/clispec/`) を別途用意して root package が組み立てる形だった。 cobra への依存が CLI 全 package に分散し、 (1) cobra の breaking change 発生時の追従が CLI 全 package に波及する、 (2) システムコマンドの責務が root / utility に分散して root package の関心事が肥大化する、 (3) 依存側プロジェクトで同等の CLI 構造を組む際に同じ utility 群を再実装する手間が発生する、 という運用上の摩擦が見えた。 §main package と CLI フレームワークの分離 / §CLI フレームワークの隠蔽方針 で cobra を `<module>/lib/clix/` wrapper package に集約し、 内部 endpoint をシステムコマンドとして wrapper が NewRoot 時に自動登録する規約を導入した。 cli module の `internal/clispec/` は廃止し、 spec emission 実装を `cli/lib/clix/` に移した。
- 当初は wrapper package を CLI 実装 module 自身の `<module>/lib/clix/` に置く規約だった (上記の cli/internal/clispec/ からの移行で導入)。 配布対象 library module を release 経路に追加 ([FLM_FEA_0004](../feature/FLM_FEA_0004__release_policy.md)) するに伴い `lib` module を新設して wrapper を library module の export root 直下 (`<lib_module>/clix/`) に再配置し、 cli module は自身に wrapper を持たず lib module の `clix` package を import する形に変えた。 これにより cobra 依存箇所が flame 全体で 1 module (lib) に集約され、 CLI 実装 module 側で wrapper を重複保持しない構造になった。
