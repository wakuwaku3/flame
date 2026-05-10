#!/usr/bin/env bash
# .github/workflows/wf__check__diff.yaml (実体層、 leaf = 合成先を持たず
# inline で `flame ci check detect` を呼ぶ) の dispatch 正しさを検証する
# (FLM_ENG_0003 §test)。
#
# 合成先 wf__ を持たないため uses / inputs parity は no-op (=「合成先 0 件
# でも fail を出さない」 形を assertions 側で保証している)。 inline run で
# 参照する shell の実在のみが意味のある検査軸となる。

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../.." && pwd)"

# shellcheck source=vendor/flame/.github/workflows/tests/shared/assertions.sh
source "$repo_root/vendor/flame/.github/workflows/tests/shared/assertions.sh"

target="$repo_root/.github/workflows/wf__check__diff.yaml"

assert_init
assert_referenced_scripts_exist "$target"
assert_finalize
