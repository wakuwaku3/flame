# repo root CLAUDE.md

本ファイルは repo 独自の拡張ルールを記述する stub である ([FLM_GEN_0007](vendor/flame/docs/adr/general/FLM_GEN_0007__resource_classification.md) §repo root における downstream resource の取り込み形式)。 flame self / 利用側 repository の両方で本構造を取る。

## vendor SoT の取り込み

flame の最重要ルールは vendor SoT 側 ([vendor/flame/CLAUDE.md](vendor/flame/CLAUDE.md)) に書かれている。 **AI は本ファイルを読んだら、 必ず link を辿って `vendor/flame/CLAUDE.md` を読むこと**。 Claude Code は file include 機構を持たないため、 自然言語の指示として AI が link を辿る規約とする。

## repo 独自の拡張ルール

本 repo (= flame harness の source 提供元) では downstream 配布対象 (rule / skill / workflow / config / ADR / `CLAUDE.md` / `.envrc` 等) の設定変更は **`vendor/flame/` 配下を修正する** ことを拡張ルールとする。 root 側の各種 stub (本 `CLAUDE.md` / `.envrc` 等) は vendor 取り込み + 必要なら repo 独自拡張のみを記述し、 downstream 設定そのものは記述しない。

利用側 repository では本セクションの内容を repo 固有のルール (= 利用側プロジェクトに固有の規約 / 拡張) に書き換える。
