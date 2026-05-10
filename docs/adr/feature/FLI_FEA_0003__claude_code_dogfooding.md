# flame の Claude Code dogfooding 起動経路の実装

## 背景

- flame harness は 3 チャネル分散 (Claude Code plugin / reusable workflow / vendor) で配布される ([FLM_FEA_0003](../../../vendor/flame/docs/adr/feature/FLM_FEA_0003__harness.md))
- 上記 ADR §dogfooding で「flame 自身も 3 チャネルを利用側と同じ経路で参照する」 が決定されている
- flame self の Claude Code plugin SoT は repo root の `plugins/flame/` に配置されている ([FLM_FEA_0003](../../../vendor/flame/docs/adr/feature/FLM_FEA_0003__harness.md) §チャネル A)
- flame は開発環境マネージャとして devbox + direnv を採用している ([FLM_ENG_0002](../../../vendor/flame/docs/adr/engineering/FLM_ENG_0002__devbox.md))
- Claude Code 2.1.x は plugin の有効化として (a) `/plugin marketplace add … → /plugin install …` (= git ベース marketplace 経由) と (b) CLI flag `--plugin-dir <path>` のセッション単位指定の 2 経路のみを公式に提供する。 環境変数 (例: `CLAUDE_PLUGIN_DIR`) や cwd 配下の auto-load は公式仕様として存在しない (公式 docs: <https://code.claude.com/docs/en/cli-reference.md> / <https://code.claude.com/docs/en/plugins.md>、 確認時点 2026-05-10)
- Claude Code 2.1.138 時点で directory-type marketplace + 任意の plugin source は install できない (公式 docs: relative paths は git 経由 add した marketplace でのみ解決される)
- direnv は `.envrc` 評価結果として **環境変数の export 差分のみ** を親 shell に伝播する (alias / shell function は伝播しない)

## 決定

flame self repo の Claude Code セッションは **PATH shadow した `claude` wrapper** で起動経路を一本化する。

### wrapper の配置

- 配置: repo root の `scripts/claude` (executable、 拡張子なし)
- PATH 通し: repo root `.envrc` で `PATH_add scripts` する。 direnv 経由で repo (もしくは worktree) に cd した時点で `scripts/` が PATH 先頭に入る

### wrapper の挙動

- cwd 起点で `git rev-parse --show-toplevel` を解決し、 `<repo_root>/plugins/flame/.claude-plugin/plugin.json` が存在する場合に限り real claude を `--plugin-dir <repo_root>/plugins/flame` 付きで exec する
- plugin manifest が存在しない / git repo 外の場合は素通し (= real claude をそのまま exec する)
- self が PATH 先頭に居るため自己再帰回避が必要。 wrapper 自身の dir を PATH から除外した上で `command -v claude` で real claude を解決する

## 影響

- flame self の repo (および worktree) 配下に cd した状態で素の `claude` を打つと自動で `plugins/flame` 込みのセッションが立ち上がる (= dogfooding 経路の取りこぼしを防ぐ)
- direnv evaluation のみで効くため、 devbox shell 起動は不要
- 素の `claude` を呼びたい場合は `command claude` / `\claude` / 絶対パス指定で escape する必要がある
- worktree 内で session を起動した場合は worktree 直下の `plugins/flame` を読み、 root で起動した場合は root の `plugins/flame` を読む (= cwd の git context に従う)
- worktree に `plugins/flame` が存在しない場合は plugin 抜きで素通しされる
- wrapper script は shellcheck / shebang 規約等の通常の shell script 静的検査の対象になる ([FLM_APP_0002](../../../vendor/flame/docs/adr/application/FLM_APP_0002__shell_script.md))
- 本 ADR は flame self の internal ADR ([FLI_GEN_0001](../general/FLI_GEN_0001__adr_prefix.md))。 利用側 repo には配布されない

## 評価

代替案として以下を検討した。

- **alias を `.envrc` に書く**: direnv は env var の export 差分しか伝播せず alias / shell function は伝播しない仕様のため不可
- **環境変数で `--plugin-dir` を指定**: Claude Code 2.1.138 公式 docs に該当 env var (例: `CLAUDE_PLUGIN_DIR`) は存在しない。 cwd 配下の auto-load 機構も公式に存在しない (確認時点 2026-05-10)
- **devbox `shell.init_hook` で alias を仕込む**: devbox shell に入った時のみ効く。 direnv で cd しただけのとき (= devbox shell 外) では効かない非対称が出る。 本 repo は devbox shell に閉じこめずに direnv 経由 cd 時に効かせる運用を採るため不採用
- **別名 wrapper (例: `flame-claude.sh`) を PATH 経由公開**: 開発者が dogfooding 経路を意識的に選ぶ必要があり、 素の `claude` を打って plugin 抜きのセッションが立ち上がる事故が起きる。 透過注入で dogfooding 経路を強制する方を採用した
- **Claude Code marketplace + `/plugin install` 経路を flame self でも採用**: 利用側 repo はこの経路を使うが、 flame self では `plugins/flame/` 自身が SoT のため自分で自分を install する不自然な構成になる。 flame self は `flame.yaml.harness.ignore: [.claude/plugins]` で marketplace 登録を skip する宣言を持つ ([FLM_FEA_0003](../../../vendor/flame/docs/adr/feature/FLM_FEA_0003__harness.md) §flame.yaml) ため当該経路は採用しない
- **wrapper 内部で `exec claude --plugin-dir …` をそのまま書く**: PATH shadow している都合上、 unqualified `claude` は wrapper 自身に解決されて無限再帰する。 self_dir を PATH から除外した上で `command -v claude` で real claude を解決する経路に倒した

過去に採用していた決定として以下の経緯がある。

- 当初は `scripts/flame-claude.sh` という別名 wrapper を PATH 経由で公開していた。 開発者が `flame-claude.sh …` と打つことで plugin 込みのセッションが立ち上がる構成だったが、 素の `claude` を打って plugin 抜きセッションが立ち上がる事故 (dogfooding 経路の取りこぼし) が運用上の懸念として残っていた。 本 ADR で `claude` 自体を PATH shadow する形に置き換えて取りこぼしを防ぐ方を採用した
