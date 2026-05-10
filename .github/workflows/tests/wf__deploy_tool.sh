#!/usr/bin/env bash
# .github/workflows/wf__deploy_tool.yaml (実体層、 leaf = 合成先を持たず
# inline で `flame ci release tool` 等を呼ぶ) の dispatch 正しさを検証する
# (FLM_ENG_0003 §test)。

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../.." && pwd)"

# shellcheck source=vendor/flame/.github/workflows/tests/shared/assertions.sh
source "$repo_root/vendor/flame/.github/workflows/tests/shared/assertions.sh"

target="$repo_root/.github/workflows/wf__deploy_tool.yaml"

assert_init
assert_referenced_scripts_exist "$target"
assert_finalize
