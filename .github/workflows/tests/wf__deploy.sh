#!/usr/bin/env bash
# .github/workflows/wf__deploy.yaml (実体層、 fan-out = wf__deploy_tool /
# wf__deploy_lib への並列 dispatch) の dispatch 正しさを検証する
# (FLM_ENG_0003 §test)。

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../.." && pwd)"

# shellcheck source=vendor/flame/.github/workflows/tests/shared/assertions.sh
source "$repo_root/vendor/flame/.github/workflows/tests/shared/assertions.sh"

target="$repo_root/.github/workflows/wf__deploy.yaml"

assert_init
assert_uses_targets_exist "$target"
assert_inputs_parity "$target"
assert_referenced_scripts_exist "$target"
assert_finalize
