#!/usr/bin/env bash
# .github/workflows/wf__check.yaml (実体層、 fan-out 合成 = wf__check__diff
# と wf__label__path を呼ぶ) の dispatch 正しさを検証する (FLM_ENG_0003
# §test)。 wf__ は workflow_call / workflow_dispatch を入口に持ち、 act の
# --list 検証は GitHub event ではなく workflow trigger 単体 parse にしか
#対応しないため、 act 検査は行わず、 静的に検証可能な観点のみを回す:
#   1. 合成先 wf__ の uses target が repo-local で実在すること
#   2. 合成先 wf__ への with の キー集合が整合すること
#   3. inline run で参照する .github/scripts/<rel>.sh が実在すること

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../.." && pwd)"

# shellcheck source=vendor/flame/.github/workflows/tests/shared/assertions.sh
source "$repo_root/vendor/flame/.github/workflows/tests/shared/assertions.sh"

target="$repo_root/.github/workflows/wf__check.yaml"

assert_init
assert_uses_targets_exist "$target"
assert_inputs_parity "$target"
assert_referenced_scripts_exist "$target"
assert_finalize
