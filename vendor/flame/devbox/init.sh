#!/usr/bin/env bash
# devbox shell init_hook の本体 (FLM_FEA_0003)。
# flame self は public 配信 (FLI_FEA_0001) のため install スクリプトを raw URL から
# anonymous fetch して bash に流す。 取得元の `<owner>/<repo>` は利用側 repo の
# `flame.yaml.harness.source` (= `github.com/<owner>/<repo>` 形式) から動的に解決する
# ことで fork / mirror 配信先からも本 init_hook が機能する。

set -euo pipefail

# CI では既に `Install flame CLI` step (`go build` from PR head、 もしくは
# `cli/scripts/install.sh` 経由の release asset 取得) が flame を `~/.local/bin/`
# に配置済の状態で `devbox run` を呼ぶ。 ここで init_hook が再度 install.sh を
# 走らせると PR head から build した flame バイナリを release 版で上書きしてしまう
# ため、 既に flame が PATH 上にある場合は skip する (idempotent 化)。
if command -v flame >/dev/null 2>&1; then
  exit 0
fi

repo_root="$(git rev-parse --show-toplevel)"
source_line=$(awk '/^[[:space:]]+source:/ { print $2; exit }' "${repo_root}/flame.yaml")
case "$source_line" in
  github.com/*)
    owner_repo="${source_line#github.com/}"
    ;;
  *)
    echo "vendor/flame/devbox/init.sh: flame.yaml の harness.source が github.com/<owner>/<repo> 形式ではない: ${source_line}" >&2
    exit 1
    ;;
esac

curl -fsSL \
  "https://raw.githubusercontent.com/${owner_repo}/HEAD/cli/scripts/install.sh" \
  | bash
