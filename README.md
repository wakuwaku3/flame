# flame

新規アプリケーション開発で毎回 0 から定義される基本ルール (ディレクトリ構成、 コーディング規約、 ビルド設定、 品質保証ループ等) を再利用可能な形で提供し、 開発者体験 (DX) を改善することを目的としたフレームワーク。 flame という名前は frame に由来する。

flame は AI エージェントとの協働開発を前提に設計されており、 AI が読み書きしやすい構造・自律的にフィードバックを得られる仕組み・明示的な技術判断記録 (ADR) を中心に構成されている。 基本思想は [FLM_GEN_0002](vendor/flame/docs/adr/general/FLM_GEN_0002__flame.md) を参照。

## flame CLI のインストール

flame CLI バイナリは GitHub Releases から配布する ([FLI_FEA_0001](docs/adr/feature/FLI_FEA_0001__github_release.md))。

```bash
# 最新版を取得
curl -fsSL https://raw.githubusercontent.com/wakuwaku3/flame/main/cli/scripts/install.sh | bash

# 任意のバージョンを取得
curl -fsSL https://raw.githubusercontent.com/wakuwaku3/flame/main/cli/scripts/install.sh | bash -s -- 1.0.0
```

flame の private fork / mirror から install する場合は、 `GITHUB_TOKEN` env を export するか [GitHub CLI](https://cli.github.com/) (`gh auth login`) で認証済の環境で実行する。 install スクリプトが token を解決して Authorization header に付加し、 同じ asset id 経由経路で取得する。

インストール先のデフォルトは `$HOME/.local/bin` で、 `FLAME_INSTALL_DIR` 環境変数で上書きできる。 インストール先が `PATH` に含まれていない場合は shell rc に追加する。 利用 shell 向けの completion ファイル (bash / zsh / fish) は XDG Base Directory 配下に同時配置される (`FLAME_NO_COMPLETION=1` で抑止可)。

## 他リポジトリへの harness 導入

flame CLI を install 後 (上記「flame CLI のインストール」)、 利用側 repo の root で以下を実行する。

```bash
flame install
```

詳細は [FLM_FEA_0003](vendor/flame/docs/adr/feature/FLM_FEA_0003__harness.md) を参照。

## flame 自身を開発する場合

flame self の開発に参加する場合の開発前提・セットアップ・開発フロー詳細は [docs/reference/developer.md](docs/reference/developer.md) を参照。
