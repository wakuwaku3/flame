# flame CLI の実装

## 背景

- flame は AI 開発における品質保証 harness を提供する ([FLM_GEN_0002](../../../vendor/flame/docs/adr/general/FLM_GEN_0002__flame.md))
- flame の品質保証は (1) AI ターン内 hook、 (2) CI、 (3) 監視の 3 層で構成される ([FLM_GEN_0003](../../../vendor/flame/docs/adr/general/FLM_GEN_0003__feedback_loop.md))
- 静的検査は checker を単位として組み立て、 hook 層と CI 層で同一の検査実装を共有する ([FLM_FEA_0001](../../../vendor/flame/docs/adr/feature/FLM_FEA_0001__checker.md))
- flame は AI 開発 harness として Claude Code を採用しており、 hook (Stop / PreToolUse) を AI ターン終端 / 直前の検査経路として持つ ([FLM_ENG_0001](../../../vendor/flame/docs/adr/engineering/FLM_ENG_0001__claude_code.md))
- flame は CI を GitHub Actions ワークフロー上に整備する ([FLM_ENG_0003](../../../vendor/flame/docs/adr/engineering/FLM_ENG_0003__github_actions.md))
- flame は開発環境マネージャとして devbox + direnv を採用する ([FLM_ENG_0002](../../../vendor/flame/docs/adr/engineering/FLM_ENG_0002__devbox.md))
- flame は Go を主開発言語として採用し、 main package を `<module>/cmd/<app_dir>/main.go` に固定する ([FLM_APP_0007](../../../vendor/flame/docs/adr/application/FLM_APP_0007__go.md))
- 配布対象アプリケーションには `_tool` suffix を付け、 配布対象でない main package とディレクトリ命名で区別する ([FLM_APP_0007](../../../vendor/flame/docs/adr/application/FLM_APP_0007__go.md) §配置)
- Go CLI 実装の基本ルール (cobra wrapper、 公開 struct 最小化、 ex による stacktrace 等) が定まっている ([FLM_APP_0008](../../../vendor/flame/docs/adr/application/FLM_APP_0008__cli.md))
- flame は配布対象 main package を GitHub Release で配布し、 release notes / cross build / install 経路を整備している ([FLI_FEA_0001](FLI_FEA_0001__github_release.md))
- 補助処理を集約する CLI の一般 policy (責務範囲・サブコマンド分割軸・shell の例外領域・新規追加経路) は [FLM_FEA_0005](../../../vendor/flame/docs/adr/feature/FLM_FEA_0005__cli_surface.md) を base policy とする
- 本 ADR は [FLM_FEA_0005](../../../vendor/flame/docs/adr/feature/FLM_FEA_0005__cli_surface.md) を満たす flame 固有の CLI 実装について flame-internal な選択を記録する

## 決定

flame CLI (= flame コマンド、 配布対象 single binary) を [FLM_FEA_0005](../../../vendor/flame/docs/adr/feature/FLM_FEA_0005__cli_surface.md) に従う flame の補助処理集約 CLI として実装する。 本 ADR は base policy 上で確定する flame 固有の選択のみを述べる。

### flame の責務カテゴリ具体 list

flame CLI が集約する補助処理の責務カテゴリ ([FLM_FEA_0005](../../../vendor/flame/docs/adr/feature/FLM_FEA_0005__cli_surface.md) §責務範囲) は flame では以下の 4 種を持つ。 これらが [FLM_FEA_0005](../../../vendor/flame/docs/adr/feature/FLM_FEA_0005__cli_surface.md) §サブコマンド体系の分割軸 にいう「責務カテゴリでの上位グルーピング」 の具体集合となる。

- **静的検査**: [FLM_FEA_0001](../../../vendor/flame/docs/adr/feature/FLM_FEA_0001__checker.md) の checker 本体実装と、 変更ファイルから (checker, target) ペアへの解決 (classifier)
- **AI hook**: Claude Code の Stop / PreToolUse hook の本体実装。 hook が emit する block / approve 等の判定ロジック
- **CI 補助**: GitHub Actions ワークフロー yaml に inline で書ききれない処理 (変更ファイル抽出、 結果集約、 release notes 生成、 配布対象 enumerate、 path-based label 付与等)
- **devbox 補助**: devbox 環境の自己注入 (activate) や初回セットアップ (init)

### 実装規約

- flame CLI は Go ([FLM_APP_0007](../../../vendor/flame/docs/adr/application/FLM_APP_0007__go.md)) で実装し、 cobra wrapper をはじめとする CLI 実装の基本ルール ([FLM_APP_0008](../../../vendor/flame/docs/adr/application/FLM_APP_0008__cli.md)) に従う
- 配布対象であるため [FLM_APP_0007](../../../vendor/flame/docs/adr/application/FLM_APP_0007__go.md) §配置 に従い `_tool` suffix を持つ main package として `cli/cmd/flame_tool/` 配下に配置する
- 配布は GitHub Release 経路 ([FLI_FEA_0001](FLI_FEA_0001__github_release.md)) を共有する。 release 起動契機 / tag 命名 / asset 形式 / install スクリプト経路は同 ADR の tool 系規約をそのまま適用する

### flame self での hook 設定

[FLM_FEA_0005](../../../vendor/flame/docs/adr/feature/FLM_FEA_0005__cli_surface.md) §shell が許される例外 で許容される trampoline を flame self では持たず、 Claude Code hook 設定 (`.claude/settings.json` および `.claude-plugin/hooks/hooks.json`) は **flame バイナリを直接起動する** 形を取る。 flame が PATH 解決可能であることを前提にできるのは flame self が source 提供元 ([FLM_GEN_0007](../../../vendor/flame/docs/adr/general/FLM_GEN_0007__resource_classification.md) §source 提供元の判定) であり、 hook 設定と flame バイナリのバージョン整合を repository 内で同期できるためである。 hook 仕様が shell 起動を強制する箇所では薄い trampoline shell が flame に処理を委譲する形で介在しうるが、 判定ロジックは flame CLI 側に置く。

## 影響

- flame self の補助処理は flame CLI のサブコマンドに集約される。 既存 `scripts/` および `.github/scripts/` 配下にあった shell スクリプト群 (静的検査・hook 本体・CI 補助・devbox 補助) は flame CLI に移行済みで、 残存する shell は [FLM_FEA_0005](../../../vendor/flame/docs/adr/feature/FLM_FEA_0005__cli_surface.md) §shell が許される例外 で許容される bootstrap (`cli/scripts/install.sh` 等) と外部 hook 仕様で起動が強制される trampoline に範囲が縮んでいる
- flame CLI のリリース ([FLI_FEA_0001](FLI_FEA_0001__github_release.md)) が CI / hook 経路の前提になる。 release 前 / install 前の環境では bootstrap shell が flame バイナリ取得経路として機能する
- GitHub Actions ワークフローは workflow yaml + flame CLI の起動の組み合わせで構成され、 yaml 内 inline shell に書かれていた処理 (env 検査・jq パース・結果集約等) は flame CLI 側に置かれる
- Claude Code hook 設定 (`.claude/settings.json` / `.claude-plugin/hooks/hooks.json`) は flame バイナリを直接起動する形になる
- [FLM_APP_0002](../../../vendor/flame/docs/adr/application/FLM_APP_0002__shell_script.md) の適用対象は flame self においても bootstrap / trampoline に縮む。 既存 shell の lint 設定 (shellcheck 等) は維持し、 残存する shell に対して引き続き機能する
- flame CLI のサブコマンドは Go の単体テスト ([FLM_APP_0009](../../../vendor/flame/docs/adr/application/FLM_APP_0009__test.md)) の対象になる。 内部関数を package private な単位で検証でき、 service-level test とは別レイヤで品質を担保できる
- 補助処理の発見性が `flame --help` に集約される。 開発者は scripts/ ツリーを走査せずに、 1 つの help ツリーで全体の補助処理を把握できる
- 同種ロジック (変更ファイル抽出、 種別判定、 jq 依存の入力 parse 等) を Go の package として共有でき、 多経路重複が解消する
- flame バイナリ自体の起動コスト (Go process startup + cobra parse) が hook / CI の各起動に乗る。 hook 起動回数の多い経路では応答時間に影響が出る

## 評価

代替案として以下を検討した。

- **補助処理ごとに独立した Go single binary を作る (例: `flame-check` / `flame-hook` / `flame-ci` を別 binary)**: 1 binary が小さく保たれる利点がある。 一方、 (1) 配布対象が複数になり release / install / version 整合の手順が肥大化する、 (2) 共通基盤 (clix wrapper、 ex stacktrace、 内部ロジック package) を多 binary で共有することになり import 構造が複雑化する、 (3) 開発者が「何ができるか」を 1 コマンドの help で発見できなくなる、 という不利益がある。 1 single binary (flame) のサブコマンド体系として集約する方を採用した。
- **flame CLI に業務アプリも含めて 1 binary に統合する**: 将来業務アプリを書くときに 1 binary でまとめる構成。 一方、 (1) flame は AI 開発の品質保証 harness としてのスコープを持ち、 業務アプリとは依存方向と release cycle が異なる、 (2) 業務アプリは独立した配布対象 (例: web サーバ・daemon) を持つことが想定される、 (3) [FLM_APP_0007](../../../vendor/flame/docs/adr/application/FLM_APP_0007__go.md) §配置 が main package を `cmd/<app_dir>/` に複数並列で置く規約をすでに持っており、 配布対象ごとに 1 main package を切る方が当該規約と整合する、 という不利益がある。 flame CLI は補助処理に責務を絞る方を採用した。
- **flame self の Claude Code hook で trampoline shell を介在させる**: hook 設定と flame バイナリのバージョン整合を shell 側で吸収できる利点がある。 一方、 (1) flame self は source 提供元であり flame バイナリの取得経路を repository 内で完結できる、 (2) trampoline を挟むと判定ロジック以外の制御 (env 解決・引数組み立て等) が shell に滲み出やすい、 という不利益がある。 hook 設定から flame バイナリを直接起動し、 trampoline は外部仕様で強制される最小限のみに留める方を採用した。
- **責務カテゴリの分類軸を flame self の既存 shell 構造と 1:1 対応で構成する**: 移行作業が機械的で済む利点がある。 一方、 (1) 既存 shell の分割は便宜的なもので、 サブコマンド体系として整合的に並ぶ保証がない、 (2) shell が消えた後にも CLI 体系だけが残るため、 shell の構造を引きずると不自然な階層が固定される、 という不利益がある。 [FLM_FEA_0005](../../../vendor/flame/docs/adr/feature/FLM_FEA_0005__cli_surface.md) の責務カテゴリ軸に従い、 既存 shell との対応は移行段階の過渡的な事実として扱う方を採用した。

## 過去経緯

過去に採用していた決定として以下の経緯がある。

- 当初は flame の補助処理を shell scripts (`scripts/`, `.github/scripts/`) に分散して実装していた。 1 種別あたり 1 shell の構成で見通しが良いように見えたが、 (1) bash 固有挙動 (jq 依存・process substitution・trap・quoting 規則) の script ごとの反復、 (2) 内部 API の単体テスト不在、 (3) hook 経路と CI 経路に同種ロジックが重複し変更時の同期漏れが起こる、 (4) Claude Code hook の入出力 JSON 処理を shell の jq 依存で扱う箇所が複雑化する、 という非効率が顕在化したため、 flame CLI を補助処理の集約点として位置付ける現決定に改訂した。 既存 shell の lint 設定は撤去せず、 bootstrap / trampoline に絞って適用範囲を縮める形を取る。
- 当初は本 ADR が flame CLI の責務範囲・サブコマンド分割軸・shell 例外領域・新規追加経路といった一般 policy を flame-internal な実装規約と同居させて持っていた。 同種の補助処理集約 CLI を flame 利用側 repository でも持ちうるため、 一般 policy を [FLM_FEA_0005](../../../vendor/flame/docs/adr/feature/FLM_FEA_0005__cli_surface.md) に独立させ downstream 側に移し、 本 ADR は flame self の具体実装 (責務カテゴリの具体 list / `cli/cmd/flame_tool/` への配置 / hook 設定での直接起動) のみに縮小した。
