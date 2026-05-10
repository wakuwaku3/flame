#!/usr/bin/env bash
# devbox shell init_hook の本体 (FLM_FEA_0003)。
# flame は private repo のため curl 直 fetch ができず、 認証済 gh で
# Contents API 経由に取得する。 取得元の `<owner>/<repo>` は利用側 repo の
# `flame.yaml.harness.source` (= `github.com/<owner>/<repo>` 形式) から
# 動的に解決する。 vendor SoT に固定 owner/repo を埋め込まないことで、
# fork / mirror 配信先からも本 init_hook が機能する。

set -euo pipefail

# CI では既に `Install flame CLI` step (`go build` from PR head、 もしくは
# `cli/scripts/install.sh` 経由の release asset 取得) が flame を `~/.local/bin/`
# に配置済の状態で `devbox run` を呼ぶ。 ここで init_hook が再度 install.sh を
# 走らせると PR head から build した flame バイナリを release 版で上書きしてしまう
# ため、 既に flame が PATH 上にある場合は skip する (idempotent 化)。
if command -v flame >/dev/null 2>&1; then
  exit 0
fi

# devbox は subshell を立てるとき env の一部を filter するため、 親 shell で
# `gh auth login` 済みでも `gh auth status` で見えないことがある。 同様に CI で
# `GH_TOKEN` env を渡しても devbox subshell 越しでは欠落する場合がある。 init_hook
# は「gh が利用可能なら flame CLI を idempotent install する」 best-effort 経路と
# して扱い、 認証が解決できない環境では skip する (= CI / 利用側 repo の install
# 経路は別途 GitHub Release asset 経由 (FLI_FEA_0001) で flame CLI を配置するため、
# init_hook で install できなくても全体は破綻しない)。
if ! gh auth status >/dev/null 2>&1; then
  echo "vendor/flame/devbox/init.sh: gh authentication unavailable in current shell; skipping flame CLI install (will be handled by separate install step)" >&2
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

gh api \
  -H 'Accept: application/vnd.github.raw' \
  "/repos/${owner_repo}/contents/cli/scripts/install.sh" \
  | bash
