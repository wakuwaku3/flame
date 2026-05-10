#!/usr/bin/env bash
# devbox shell init_hook の本体 (FLM_FEA_0003)。
# 取得元 repo を public 配信前提で扱い、 install スクリプトを raw URL から anonymous
# fetch して bash に流す。 取得元の `<owner>/<repo>` は利用側 repo の
# `flame.yaml.harness.source` (= `github.com/<owner>/<repo>` 形式) から動的に解決する
# ことで fork / mirror 配信先からも本 init_hook が機能する。

set -euo pipefail

# init_hook 起動時点で既に flame が install 済のケース (= 先行 install step や
# 既存の開発環境で別経路から install されている等) では、 ここで再度 install.sh
# を走らせると既存バイナリを上書きする副作用がある。 既に flame が PATH 上に
# ある場合は skip する (idempotent 化)。
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
