# flame 開発者ガイド

flame self の開発に参加する開発者向けの reference ドキュメント ([FLM_APP_0001](../../vendor/flame/docs/adr/application/FLM_APP_0001__document.md) §種別 §stock §reference)。

## 開発前提

- **[devbox](https://www.jetify.com/devbox)** — 開発ツールのバージョン pin と導入 ([FLM_ENG_0002](../../vendor/flame/docs/adr/engineering/FLM_ENG_0002__devbox.md))
- **[direnv](https://direnv.net/)** — リポジトリ移動時の devbox 環境自動アクティベーション
- **[tmux](https://github.com/tmux/tmux/wiki)** — Claude Code セッションを複数並走させるためのマルチプレクサ
- **[Claude Code](https://docs.claude.com/en/docs/claude-code)** — AI 開発 harness。開発は基本的に Claude Code 経由で行う ([FLM_ENG_0001](../../vendor/flame/docs/adr/engineering/FLM_ENG_0001__claude_code.md))

## セットアップ

```bash
git clone https://github.com/wakuwaku3/flame.git
cd flame
direnv allow         # 初回のみ。.envrc が devbox を自動起動するようになる
devbox install       # devbox.lock に従って tools を取得
```

以降、リポジトリに `cd` するだけで direnv + devbox 経由で `devbox.json` に pin された CLI が PATH に乗る。

## 開発の進め方

flame は AI 主体の開発を前提とする。 Claude Code に指示を出すと、 ハーネスが AI ターン内 hook で **静的検査 + AI レビュー** を自動実行し、 違反は同一ターン内で AI 自身が fix する (詳細: [FLM_ENG_0001](../../vendor/flame/docs/adr/engineering/FLM_ENG_0001__claude_code.md))。 開発者の主な仕事は、 提示された成果物 (diff) をレビューして方針を返すこと。

品質保証は 3 層 FB ループで構成される ([FLM_GEN_0003](../../vendor/flame/docs/adr/general/FLM_GEN_0003__feedback_loop.md))。

1. **AI ターン内 hook** — Stop hook で静的検査、 PreToolUse hook で `git push` 直前に AI レビューが走り、AI が応答を返す前に違反を fix する
2. **CI** — main マージ前の最終ゲート (hook と同一の検査も重複実行)
3. **監視** — デプロイ後検出。hook / CI に再現テストとして昇格させる

手動で検査を回す場合: `flame check <type> <file>...` (type は yaml / shell / go / json / document / adr / devbox / flow-document / github-actions のいずれか)

## 関連 ADR / リソース

- [flame の基本思想 (FLM_GEN_0002)](../../vendor/flame/docs/adr/general/FLM_GEN_0002__flame.md)
- [資産分類 (flame-internal / downstream) (FLM_GEN_0007)](../../vendor/flame/docs/adr/general/FLM_GEN_0007__resource_classification.md)
- [flame CLI の実装 (FLI_FEA_0002、 flame-internal)](../adr/feature/FLI_FEA_0002__flame_cli.md)
- [flame の GitHub Release 経路の実装 (FLI_FEA_0001、 flame-internal)](../adr/feature/FLI_FEA_0001__github_release.md)
