#!/usr/bin/env bash
# .github/workflows/flame-trg__push__main.yaml (install copy) の dispatch
# 正しさを検証する (FLM_ENG_0003 §test、 FLM_FEA_0003 §workflow の install 命名規約)。

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../../../.." && pwd)"

# shellcheck source=vendor/flame/.github/workflows/tests/shared/assertions.sh
source "$script_dir/shared/assertions.sh"

target="$repo_root/.github/workflows/flame-trg__push__main.yaml"

assert_init
assert_uses_targets_exist "$target"
assert_inputs_parity "$target"
assert_act_list "$target" "push" "$script_dir/shared/events/push.json"
assert_finalize
