#!/usr/bin/env bash
# .github/workflows/tests/<workflow_basename>.sh から source される共通
# アサーション集 (FLM_ENG_0003 §test)。 各 test script は対応する
# `.github/workflows/<basename>.yaml` 1 本に対する dispatch / parse
# 検証を、 ここで定義する関数の組合せだけで表現する。
#
# 提供 API:
#   assert_init                                — 失敗集積バッファの初期化
#   assert_uses_targets_exist <wf>             — jobs.<id>.uses が repo-local
#                                                の wf__ ファイルを指す場合に
#                                                当該 ファイルが実在するか
#                                                検証する
#   assert_inputs_parity <wf>                  — 各 jobs.<id>.with の キー
#                                                集合が呼出先 wf__ の
#                                                workflow_call.inputs と
#                                                整合 (required ⊆ with ⊆
#                                                宣言済みすべて) するか
#                                                検証する
#   assert_referenced_scripts_exist <wf>       — `bash .github/scripts/<rel>.sh`
#                                                形式で参照される shell の
#                                                実在を検証する
#   assert_act_list <wf> <event> <eventpath>   — `act <event> -W <wf>
#                                                --eventpath <eventpath>
#                                                --list` が exit 0 を
#                                                返すか検証する (docker
#                                                必須)。 docker 不在時は
#                                                SKIP。 stdout の grep は
#                                                与えたファイル自身が出る
#                                                だけのトートロジーなので
#                                                行わない
#   assert_finalize                            — 集積した FAIL を stderr に
#                                                出して exit 0/1 を伝搬

# 当ファイルは source 専用。 直接実行された場合は誤用として弾く。 source
# 経由なら ${BASH_SOURCE[0]} と $0 が異なる (前者は当ファイル、 後者は
# caller) ため当該等価判定で検出する。
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
  echo "error: $(basename -- "${BASH_SOURCE[0]}") must be sourced, not executed" >&2
  exit 64
fi

# 失敗集積。 caller が assert_init を呼んでから assert_* を順に呼ぶ。 各
# assert_* は失敗 1 件ごとに 1 行を push し関数自体は 0 終了する (set -e
# 下で連続呼出を回せるように)。
_assert_failures=()

assert_init() {
  _assert_failures=()
}

_assert_fail() {
  _assert_failures+=("FAIL: $*")
}

assert_uses_targets_exist() {
  local wf="$1"
  local repo_root
  repo_root="$(cd "$(dirname "$wf")/../.." && pwd)"

  local raw
  if ! raw=$(yq eval '.jobs[].uses // ""' "$wf" 2>/dev/null); then
    _assert_fail "$wf: jobs.uses extraction crashed"
    return 0
  fi
  local found_any=false
  while IFS= read -r want; do
    [ -z "$want" ] && continue
    found_any=true
    if [[ "$want" == ./* ]]; then
      local target="$repo_root/${want#./}"
      if [ ! -f "$target" ]; then
        _assert_fail "$wf: jobs.*.uses references missing file '$want' (resolved to '$target') (FLM_ENG_0003)"
      fi
    fi
    # 外部 reusable workflow (./.github/... で始まらない) は静的に検証不能
    # なので静かに skip する。 GitHub 側で resolve されるためローカルで
    # チェックする方法が無い。
  done <<<"$raw"
  if [ "$found_any" != "true" ]; then
    # uses を持たない workflow (= leaf wf__ で steps だけで構成されるもの)
    # に対しては no-op success とする。 inputs parity / act 検証も同様に
    # caller 側が呼ばない構成を取る。
    return 0
  fi
}

assert_inputs_parity() {
  local wf="$1"
  local repo_root
  repo_root="$(cd "$(dirname "$wf")/../.." && pwd)"

  local pairs_file
  pairs_file=$(mktemp)
  trap 'rm -f "$pairs_file"' RETURN

  if ! yq eval \
    '.jobs | to_entries | .[] | select(.value.uses) | (.key + "\t" + .value.uses)' \
    "$wf" >"$pairs_file" 2>/dev/null; then
    _assert_fail "$wf: jobs/uses pairs extraction crashed"
    return 0
  fi

  while IFS=$'\t' read -r job_id uses_path; do
    [ -z "$job_id" ] && continue
    [[ "$uses_path" != ./* ]] && continue
    local target="$repo_root/${uses_path#./}"
    [ ! -f "$target" ] && continue

    local declared required passed
    declared=$(yq eval '.on.workflow_call.inputs // {} | keys | .[]' "$target" 2>/dev/null || true)
    required=$(yq eval \
      '.on.workflow_call.inputs // {} | to_entries | map(select(.value.required == true)) | .[].key' \
      "$target" 2>/dev/null || true)
    # job_id を yq の path 式に直接展開すると `.` / `[]` 等の特殊文字を含む
    # job ID で query が壊れるため env(KEY) 経由で literal として渡す
    # (defense-in-depth)。
    passed=$(env KEY="$job_id" yq eval \
      '.jobs[env(KEY)].with // {} | keys | .[]' \
      "$wf" 2>/dev/null || true)

    local declared_sorted required_sorted passed_sorted
    declared_sorted=$(printf '%s\n' "$declared" | sed '/^$/d' | LC_ALL=C sort -u)
    required_sorted=$(printf '%s\n' "$required" | sed '/^$/d' | LC_ALL=C sort -u)
    passed_sorted=$(printf '%s\n' "$passed" | sed '/^$/d' | LC_ALL=C sort -u)

    local missing extras
    missing=$(LC_ALL=C comm -23 <(echo "$required_sorted") <(echo "$passed_sorted"))
    extras=$(LC_ALL=C comm -23 <(echo "$passed_sorted") <(echo "$declared_sorted"))

    if [ -n "$missing" ]; then
      local missing_json
      missing_json=$(printf '%s\n' "$missing" | jq -Rsc 'split("\n") | map(select(length > 0))')
      _assert_fail "$wf: job '$job_id': missing required input(s) for $uses_path: $missing_json (FLM_ENG_0003)"
    fi
    if [ -n "$extras" ]; then
      local extras_json
      extras_json=$(printf '%s\n' "$extras" | jq -Rsc 'split("\n") | map(select(length > 0))')
      _assert_fail "$wf: job '$job_id': passes input(s) not declared by $uses_path: $extras_json (FLM_ENG_0003)"
    fi
  done < "$pairs_file"
}

assert_referenced_scripts_exist() {
  local wf="$1"
  local repo_root
  repo_root="$(cd "$(dirname "$wf")/../.." && pwd)"
  # `run:` block 内に登場する `bash .github/scripts/<rel>.sh` (および同様の
  # 経路) を抽出して実在検証する。 引数は除いた path 部のみを取り、 同一
  # path の重複は uniq でまとめる。 run block を base64 経由で受けとってから
  # grep する経路は assertion を簡素化する利点があるが、 実用上 run block の
  # シェル本体は inline で grep 可能なため当ファイル内テキストを直接 scan
  # する形を取る。
  local refs
  refs=$(grep -oE '\.github/scripts/[A-Za-z0-9_./-]+\.sh' "$wf" | LC_ALL=C sort -u || true)
  while IFS= read -r ref; do
    [ -z "$ref" ] && continue
    if [ ! -f "$repo_root/$ref" ]; then
      _assert_fail "$wf: referenced script '$ref' does not exist (FLM_ENG_0003 §inline shell の制限)"
    fi
  done <<<"$refs"
}

# act の動的検証は docker 必須。 未インストール / daemon 不通時は SKIP し
# stderr に 1 行残す。 strict mode は呼出側 (caller test script) で必要なら
# 環境変数で指定可能だが、 default は lenient (docker 不在で SKIP / exit 0)。
_docker_available() {
  command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1
}

assert_act_list() {
  local wf="$1" event="$2" eventpath="$3"
  if ! _docker_available; then
    echo "SKIP: $wf: docker is unavailable; skipping 'act --list' assertion" >&2
    return 0
  fi
  local out
  if ! out=$(act "$event" -W "$wf" --eventpath "$eventpath" --list 2>&1); then
    _assert_fail "$wf: 'act --list' failed for event '$event'; output below"
    printf '%s\n' "$out" >&2
  fi
}

assert_finalize() {
  if [ "${#_assert_failures[@]}" -eq 0 ]; then
    return 0
  fi
  for line in "${_assert_failures[@]}"; do
    printf '%s\n' "$line" >&2
  done
  exit 1
}
