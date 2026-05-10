# 補助処理 CLI の公開 surface: 責務範囲・サブコマンド体系・shell 例外

## 背景

- 補助処理を提供する harness は、 開発フィードバックループ ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md)) における (1) AI ターン内 hook、(2) CI、(3) 監視 の各層から再利用される処理を持つ
- 静的検査は checker を単位として組み立て、 hook 層と CI 層で同一の検査実装を共有する ([FLM_FEA_0001](FLM_FEA_0001__checker.md))
- 補助処理を提供する harness の利用側 (downstream) は、 当該 harness が提供する補助処理を hook / CI / 環境管理 等の経路から呼び出す消費者として位置付く
- 補助処理を提供する形態は概ね次の 3 系統が考えうる:
  - shell スクリプト群を配布して downstream で直接実行させる
  - 補助処理ごとに独立した binary を複数配布する
  - 補助処理を 1 つの single binary に集約してサブコマンド体系で公開する
- 補助処理は時間とともに増減し、 hook 経路と CI 経路の両方で同種ロジック (例: 変更ファイル抽出、 入力 JSON parse、 結果集約) が要求される
- AI hook の入出力 (Claude Code の hook が JSON で受け渡す tool_input、 hook が emit する `decision` JSON 等) や CI の入出力 (GitHub Actions の matrix output、 path-based label 付与等) を扱う必要があり、 補助処理側に構造化データ処理が要求される
- 一部の経路では、 補助処理 CLI 本体が起動する前段で動かなければならない処理 (= 当該 CLI 自体の install 処理) や、 外部 hook 仕様で shell の起動が強制される最小起動口 (= trampoline) が存在する
- 補助処理 CLI を提供する harness は、 利用側プロジェクトに対して「公開 surface としてどこを叩かせるか」 を明示する必要がある (内部実装ではなく、 利用者から見たコマンド表面)

## 決定

補助処理 CLI を提供する harness は、 当該 CLI を **配布対象 single binary としてのサブコマンド体系** という形で公開 surface を構成する。 利用側 (downstream) は当該 CLI を「利用者が叩くコマンド」 として消費し、 内部実装には踏み込まない。

### 責務範囲

補助処理 CLI は **harness が提供する補助処理を集約する単一 entrypoint** として位置付ける。 当該 CLI の責務は以下に絞る。

- hook / CI / 環境管理 等の **補助処理** を提供する
- 配布対象 single binary として 1 つにまとまり、 利用者は 1 コマンドの help でその全体像を把握できる

以下は補助処理 CLI の責務に **含まない**。

- 当該 CLI バイナリ取得より前に動く **bootstrap 処理** (= 当該 CLI を install する処理)
- 外部 hook 仕様で shell の起動を強制される **trampoline 部分** (例: hook 起動口が `command` 文字列を bash で起動する制約)
- harness を利用するプロジェクト側の **業務ロジック** (= harness は補助処理を提供する立場であり、 業務アプリではない)
- 配布形態の異なる成果物 (例: web アプリ・daemon 等は別の独立した配布対象として構成する)

### サブコマンド体系の分割軸

補助処理 CLI のサブコマンド体系は **責務カテゴリ** を上位の分割軸とする。 既存実装 (移行前の shell スクリプト構成や個別ツール構成) との 1:1 対応で構造を引きずらず、 責務カテゴリで上位グルーピングを構成し、 その配下で個別処理をサブコマンドとして並べる。 具体的なカテゴリ名・コマンド名は本 ADR の対象外とし、 各 harness の実装側で決める。

### shell が許される例外

補助処理 CLI が責務とする処理を shell スクリプトで実装することは原則認めない。 例外として shell が許されるのは以下の 2 種に限定する。

- **bootstrap**: 補助処理 CLI バイナリが install される前段の処理。 当該 CLI コマンドが PATH に解決できない時点で動くものに限る
- **trampoline**: 外部 hook 仕様で shell の起動が強制される最小の起動口。 hook 起動口から補助処理 CLI バイナリへの引き渡しに必要な最小限の処理のみを含み、 判定ロジック・データ変換は CLI 側に置く

上記例外領域に新規 shell を追加する場合は引き続き shell スクリプトの基本ルール ([FLM_APP_0002](../application/FLM_APP_0002__shell_script.md)) を適用する。

### 新規補助処理追加時の経路

補助処理を新規追加する場合、 まず補助処理 CLI のサブコマンドとして実装する。 既存責務カテゴリのいずれかに属さない新カテゴリを追加する場合は、 サブコマンドの上位グルーピングを 1 つ追加する。 新規補助処理を shell として追加することは bootstrap / trampoline 例外に該当する場合を除いて認めない。

### 公開 surface の境界

補助処理 CLI を利用する側 (downstream) は、 当該 CLI を **利用者が叩くコマンド** として消費する。 公開 surface は以下に限る。

- サブコマンドの体系 (上位グルーピングおよび個別サブコマンド名)
- 各サブコマンドの flag / 引数
- サブコマンドの exit code 規約・標準出力 / 標準エラー出力フォーマット (機械可読を前提とする経路では JSON 等の安定形式)

内部の実装言語・package 構造・モジュール構成は公開 surface に含まない。 利用側は内部実装に依存せず、 サブコマンド呼び出し経由で補助処理を消費する。

## 影響

- 利用側プロジェクトは hook / CI / 環境管理 等から補助処理 CLI のサブコマンドを直接起動する形になる。 hook 設定や CI ワークフロー yaml に inline で書かれていた処理 (env 検査・jq パース・結果集約等) は補助処理 CLI 側に移される
- 利用側のフィードバックループ ([FLM_GEN_0003](../general/FLM_GEN_0003__feedback_loop.md)) の各層は、 補助処理 CLI のサブコマンドを共通の入口として共有する。 同種ロジックが hook 経路と CI 経路に重複実装される事態を、 CLI 側に集約することで防げる
- 補助処理の発見性が `<cli> --help` に集約される。 利用側は scripts ツリーや独立 binary 群を走査せずに、 1 つの help ツリーで全体の補助処理を把握できる
- 静的検査 ([FLM_FEA_0001](FLM_FEA_0001__checker.md)) の checker 実装も補助処理 CLI のサブコマンドとして配置でき、 hook 層と CI 層で同一実装を共有する形が CLI 起動の単一経路に揃う
- bootstrap / trampoline 例外領域は引き続き shell として残存し、 [FLM_APP_0002](../application/FLM_APP_0002__shell_script.md) の適用対象になる。 例外領域に新規 shell を追加する場合は当該 ADR を適用する
- 補助処理 CLI 自体の install / version 整合は当該 CLI 1 binary 分で済み、 多 binary 構成で発生する release / version 整合の手順肥大化を回避できる
- 利用側は補助処理 CLI の公開 surface (サブコマンド・flag) のみに依存し、 内部実装変更による破壊を受けにくい。 内部 refactor は公開 surface を維持する限り downstream に伝播しない
- 本 ADR は依存側プロジェクトへ伝播する (本 ADR は FEA カテゴリかつ downstream のため)

## 評価

代替案として以下を検討した。

- **補助処理を shell スクリプト群で配布し、 downstream で直接実行させる**: 学習コストが低く、 軽い処理は shell で完結する利点がある。 一方、 (1) bash 固有の挙動 (jq 依存・process substitution・trap・quoting 規則) を script ごとに反復する重複コストが大きい、 (2) 静的型が無く内部 API の単体テストが書けない、 (3) 同種の処理ロジック (変更ファイル抽出・入力 JSON parse 等) が hook 経路と CI 経路に重複実装され同期漏れの温床になる、 (4) downstream が補助処理を消費するために shell スクリプトの実体配置に依存することになり、 公開 surface が「ファイルパス + 引数」 という低レベルなものに固定される、 という不利益がある。 single binary に集約する方を採用した。
- **補助処理ごとに独立した single binary を作る (例: 静的検査 / hook / CI 補助 を別 binary)**: 1 binary が小さく保たれる利点がある。 一方、 (1) 配布対象が複数になり release / install / version 整合の手順が肥大化する、 (2) 共通基盤 (CLI wrapper、 入力 JSON parse、 結果集約等の内部ロジック) を多 binary で共有することになり依存構造が複雑化する、 (3) 利用者が「当該 harness で何ができるか」 を 1 コマンドの help で発見できなくなる、 という不利益がある。 1 single binary のサブコマンド体系として集約する方を採用した。
- **補助処理 CLI に業務アプリも含めて 1 binary に統合する**: 将来業務アプリを書くときに 1 binary でまとめる構成。 一方、 (1) 補助処理 CLI は harness としてのスコープを持ち、 業務アプリとは依存方向と release cycle が異なる、 (2) 業務アプリは独立した配布対象 (例: web サーバ・daemon) を持つことが想定される、 (3) 配布対象ごとに 1 main package を切る方が一般的な配布規約と整合する、 という不利益がある。 補助処理 CLI は補助処理に責務を絞る方を採用した。
- **shell を完全禁止する**: 純粋に CLI バイナリのみで構成する形。 一方、 (1) 補助処理 CLI を install する処理は当該バイナリが手元に無い段階で動く必要があり CLI バイナリでは bootstrap できない、 (2) 外部 hook 仕様 (Claude Code の Bash hook 等) が shell の起動を強制する箇所では薄い trampoline が不可避、 という不利益がある。 bootstrap / trampoline の 2 例外を残す方を採用した。
- **サブコマンド体系を既存実装 (shell scripts や個別ツール) と 1:1 対応で構成する**: 移行作業が機械的で済む利点がある。 一方、 (1) 既存実装の分割は便宜的なもので、 サブコマンド体系として整合的に並ぶ保証がない、 (2) 将来既存実装が消えた後にも CLI 体系だけが残るため、 既存実装の構造を引きずると不自然な階層が固定される、 という不利益がある。 サブコマンド体系の上位軸を責務カテゴリで切り、 既存実装との対応は移行段階の過渡的な事実として扱う方を採用した。
- **shell を残したまま補助処理 CLI を「shell の薄い wrapper」 として並走させる**: shell 実体は維持したまま CLI 表面を整える構成。 hook / CI 設定の差し替えが軽い利点がある。 一方、 (1) wrapper が永続的に二重化したコード経路を生む、 (2) shell が単体テスト可能になるわけではなく、 静的型不在・重複実装の課題は解消しない、 (3) 補助処理 CLI 側の責務範囲が「shell の表面」 に縛られて意味的整理が進まない、 という不利益がある。 shell 実体を CLI 側に巻き取る形で移行し、 wrapper は移行過渡期の暫定にとどめる方を採用した。
- **公開 surface に内部実装 (package 構造・関数 API 等) を含める**: 利用側が内部関数を直接呼び出せる柔軟さがある。 一方、 (1) 内部 refactor が downstream を破壊しうる、 (2) 公開 surface が膨らんで harness 側の変更自由度が失われる、 という不利益がある。 公開 surface はサブコマンド・flag・exit code・出力フォーマットに限定し、 内部実装は非公開とする方を採用した。

過去に採用していた決定として以下の経緯がある。

- 本 ADR は FLI_FEA_0002 (flame CLI) から CLI 公開 surface に関する policy 部分を独立させた経緯を持つ。 FLI_FEA_0002 は当初、 補助処理 CLI を提供する一般 policy (= 配布対象 single binary を補助処理の集約点とする方針、 責務範囲、 サブコマンド体系の分割軸、 shell 例外、 新規追加経路、 公開 surface 境界) と、 flame harness 自身の internal な実装規約 (採用言語・wrapper 選択・main package 配置・release 経路の共有等) が 1 ADR に混在していた。 downstream に伝播すべきは前者 (公開 surface 一般 policy) であり、 後者 (flame self の実装規約) は internal にとどまるべき内容のため、 FLI_FEA_0002 は internal 部分のみを残す形に縮小し、 downstream へ伝播する一般 policy を本 ADR として独立させた。
