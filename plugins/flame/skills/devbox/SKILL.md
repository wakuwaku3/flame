---
name: devbox
description: devbox.json を変更する工程で起動する。devbox.lock の再生成と両ファイルの同一コミット化まで完了させる。
---

# devbox skill

参照 ADR: [FLM_ENG_0002: 開発環境マネージャとして devbox + direnv を採用する](../../../../vendor/flame/docs/adr/engineering/FLM_ENG_0002__devbox.md)

## 手順

ADR の規約に従ったうえで、本 skill が起動した工程では以下の procedural を完了させる。

- `devbox.json` を変更したら、続けて devbox の install / update コマンドを実行し `devbox.lock` を再生成する
- `devbox.json` と `devbox.lock` は同一コミットに含める
- 新規パッケージを追加した場合、direnv reload またはシェル再起動で当該パッケージが PATH に乗り実行可能であることを動作確認する
- `init_hook` から呼ばれるスクリプトを追加・変更した場合は shell-script skill を併用する
