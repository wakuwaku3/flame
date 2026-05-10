#!/usr/bin/env bash
# .github/workflows/flame-trg__pull_request__opened_synchronize_reopened.yaml
# (install copy) の dispatch 正しさを検証する (FLM_ENG_0003 §test、
# FLM_FEA_0003 §workflow の install 命名規約)。

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../../../.." && pwd)"

# shellcheck source=vendor/flame/.github/workflows/tests/shared/assertions.sh
source "$script_dir/shared/assertions.sh"

target="$repo_root/.github/workflows/flame-trg__pull_request__opened_synchronize_reopened.yaml"

assert_init
assert_uses_targets_exist "$target"
assert_inputs_parity "$target"
assert_act_list "$target" "pull_request" "$script_dir/shared/events/pull_request.json"
assert_finalize
