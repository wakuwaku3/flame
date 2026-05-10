#!/usr/bin/env bash
# flame CLI installer (FLM_FEA_0002)。
#
# flame self は public リポジトリで配布するため anonymous で起動できる。 GITHUB_TOKEN env
# もしくは `gh auth token` で token が解決された場合は Authorization header に付加し、
# private fork / mirror 配下の release asset 取得経路にも同じ script で対応する
# (FLI_FEA_0001 §install スクリプトの配置)。

set -euo pipefail

readonly app_name="flame"
readonly repo="wakuwaku3/flame"
readonly tag_prefix="${app_name}/v"
readonly api_root="https://api.github.com/repos/${repo}"

# tmp_dir / token は trap cleanup や後続関数からの参照のため script scope に置く
# (空文字 default で `set -u` 下でも安全に展開できる)。
tmp_dir=""
token=""

cleanup() {
  if [ -n "${tmp_dir}" ] && [ -d "${tmp_dir}" ]; then
    rm -rf -- "${tmp_dir}"
  fi
}
trap cleanup EXIT

err() {
  echo "error: $*" >&2
  exit 1
}

note() {
  echo "info: $*" >&2
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    err "required command not found: $1"
  fi
}

resolve_token() {
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    printf '%s\n' "$GITHUB_TOKEN"
    return 0
  fi
  if command -v gh >/dev/null 2>&1; then
    local t
    if t="$(gh auth token 2>/dev/null)" && [ -n "$t" ]; then
      printf '%s\n' "$t"
      return 0
    fi
  fi
  return 1
}

detect_os() {
  local raw
  raw="$(uname -s)"
  case "$raw" in
    Linux) echo "linux" ;;
    Darwin) echo "darwin" ;;
    MINGW* | MSYS* | CYGWIN*) echo "windows" ;;
    *) err "unsupported OS: $raw" ;;
  esac
}

detect_arch() {
  local raw
  raw="$(uname -m)"
  case "$raw" in
    x86_64 | amd64) echo "amd64" ;;
    aarch64 | arm64) echo "arm64" ;;
    *) err "unsupported architecture: $raw" ;;
  esac
}

api_get() {
  local url="$1"
  local accept="${2:-application/vnd.github+json}"
  if [ -n "${token}" ]; then
    curl -fsSL \
      -H "Authorization: Bearer ${token}" \
      -H "Accept: ${accept}" \
      "${url}"
  else
    curl -fsSL \
      -H "Accept: ${accept}" \
      "${url}"
  fi
}

resolve_latest_version() {
  # GNU 拡張の `sort -V` は使わず POSIX field sort で代替する。
  local body versions
  if ! body="$(api_get "${api_root}/releases?per_page=100")"; then
    err "failed to query GitHub releases API"
  fi
  versions="$(printf '%s' "$body" \
    | jq -r --arg prefix "${tag_prefix}" '
        .[] | .tag_name | select(startswith($prefix)) | sub("^"+$prefix; "")
      ' \
    | sort -t. -k1,1n -k2,2n -k3,3n)"
  if [ -z "$versions" ]; then
    err "no release found for tag prefix '${tag_prefix}*' (publish a release first or pass a version explicitly)"
  fi
  printf '%s\n' "$versions" | tail -n 1
}

resolve_asset_id() {
  # asset URL ではなく asset id 経由で取り出す。 private fork / mirror 配下の release は
  # asset id + `Accept: application/octet-stream` のみが認証込みの配信経路となるため、
  # public / private 双方を単一経路で扱うため asset id を採る。
  local tag="$1"
  local asset_name="$2"
  local body asset_id
  if ! body="$(api_get "${api_root}/releases/tags/${tag}")"; then
    err "failed to fetch release metadata for tag '${tag}'"
  fi
  asset_id="$(printf '%s' "$body" \
    | jq -r --arg name "${asset_name}" '
        .assets[] | select(.name == $name) | .id
      ' \
    | head -n 1)"
  if [ -z "$asset_id" ] || [ "$asset_id" = "null" ]; then
    err "asset '${asset_name}' not found in release '${tag}'"
  fi
  printf '%s\n' "$asset_id"
}

download_asset() {
  # GitHub は signed S3 URL に 302 redirect するため `-L` で追従する。 -L 後の URL では
  # Authorization header が S3 の署名付き URL を妨害しうるが curl は cross-origin redirect 時に
  # Authorization を自動 strip する挙動のため意図通り動く。
  local asset_id="$1"
  local out_path="$2"
  if [ -n "${token}" ]; then
    curl -fsSL \
      -H "Authorization: Bearer ${token}" \
      -H "Accept: application/octet-stream" \
      "${api_root}/releases/assets/${asset_id}" \
      -o "${out_path}"
  else
    curl -fsSL \
      -H "Accept: application/octet-stream" \
      "${api_root}/releases/assets/${asset_id}" \
      -o "${out_path}"
  fi
}

archive_ext_for_os() {
  case "$1" in
    windows) echo "zip" ;;
    *) echo "tar.gz" ;;
  esac
}

binary_name_for_os() {
  case "$1" in
    windows) echo "${app_name}.exe" ;;
    *) echo "${app_name}" ;;
  esac
}

detect_shell() {
  local override="${FLAME_COMPLETION_SHELL:-}"
  if [ -n "$override" ]; then
    printf '%s\n' "$override"
    return 0
  fi
  local s="${SHELL:-}"
  case "$(basename "$s" 2>/dev/null)" in
    bash) echo "bash" ;;
    zsh) echo "zsh" ;;
    fish) echo "fish" ;;
    *) echo "" ;;
  esac
}

completion_target_for_shell() {
  local shell="$1"
  local override_dir="${FLAME_COMPLETION_DIR:-}"
  local xdg_data="${XDG_DATA_HOME:-$HOME/.local/share}"
  local xdg_config="${XDG_CONFIG_HOME:-$HOME/.config}"
  local dir file
  case "$shell" in
    bash)
      dir="${override_dir:-${xdg_data}/bash-completion/completions}"
      file="${app_name}"
      ;;
    zsh)
      dir="${override_dir:-${xdg_data}/zsh/site-functions}"
      file="_${app_name}"
      ;;
    fish)
      dir="${override_dir:-${xdg_config}/fish/completions}"
      file="${app_name}.fish"
      ;;
    *)
      return 1
      ;;
  esac
  printf '%s/%s\n' "$dir" "$file"
}

install_completion() {
  local binary="$1"
  local os="$2"
  if [ "${FLAME_NO_COMPLETION:-0}" = "1" ]; then
    note "shell completion: skipped (FLAME_NO_COMPLETION=1). 手動生成は '${app_name} completion {bash|zsh|fish|powershell} --help' を参照"
    return 0
  fi
  if [ "$os" = "windows" ]; then
    note "shell completion: windows は自動配置対象外。 PowerShell では \"${app_name} completion powershell | Out-String | Invoke-Expression\" を \$PROFILE に追加することで有効化できる"
    return 0
  fi
  local shell target target_dir
  shell="$(detect_shell)"
  if [ -z "$shell" ]; then
    note "shell completion: 検出不能 (\$SHELL='${SHELL:-}'). FLAME_COMPLETION_SHELL=bash|zsh|fish を指定して再実行することで配置できる"
    return 0
  fi
  if ! target="$(completion_target_for_shell "$shell")"; then
    note "shell completion: '${shell}' は自動配置対象外"
    return 0
  fi
  target_dir="$(dirname "$target")"
  if ! mkdir -p "$target_dir" 2>/dev/null; then
    note "shell completion: 配置先 '${target_dir}' を作成できない。 FLAME_COMPLETION_DIR で書き込み可能な dir を指定するか FLAME_NO_COMPLETION=1 で skip できる"
    return 0
  fi
  if ! "$binary" completion "$shell" >"${target}.tmp" 2>/dev/null; then
    rm -f "${target}.tmp"
    note "shell completion: '${binary} completion ${shell}' の生成に失敗 (binary が completion 未対応の可能性)。 skip"
    return 0
  fi
  mv "${target}.tmp" "$target"
  note "installed ${shell} completion to ${target}"
  if [ "$shell" = "zsh" ]; then
    note "zsh: '${target_dir}' を fpath に追加して 'compinit' を再実行することで有効化される (例: \"echo 'fpath=(${target_dir} \\\$fpath)' >> ~/.zshrc; echo 'autoload -Uz compinit && compinit' >> ~/.zshrc\")"
  fi
}

main() {
  require_cmd curl
  require_cmd uname
  require_cmd mktemp
  require_cmd install
  require_cmd jq

  if ! token="$(resolve_token)"; then
    note "no GITHUB_TOKEN / gh credential found; proceeding anonymously (private fork / mirror への install は GITHUB_TOKEN env か 'gh auth login' を要する)"
  fi

  local requested_version="${1:-}"
  local os arch version
  os="$(detect_os)"
  arch="$(detect_arch)"

  if [ -z "$requested_version" ]; then
    version="$(resolve_latest_version)"
  else
    version="${requested_version#v}"
  fi

  local install_dir="${FLAME_INSTALL_DIR:-$HOME/.local/bin}"
  local binary_name install_path
  binary_name="$(binary_name_for_os "$os")"
  install_path="${install_dir}/${binary_name}"

  if [ -x "$install_path" ]; then
    local current
    if current=$("$install_path" --version 2>/dev/null | awk '{print $2}'); then
      if [ "$current" = "$version" ]; then
        note "${app_name} ${version} is already installed at ${install_path}"
        install_completion "$install_path" "$os"
        return 0
      fi
    fi
  fi

  local ext archive_name tag asset_id
  ext="$(archive_ext_for_os "$os")"
  archive_name="${app_name}_${version}_${os}_${arch}.${ext}"
  tag="${tag_prefix}${version}"

  note "resolving asset id for ${archive_name} in release ${tag}"
  asset_id="$(resolve_asset_id "${tag}" "${archive_name}")"

  tmp_dir="$(mktemp -d)"

  note "downloading ${archive_name} (asset id ${asset_id}) via GitHub API"
  if ! download_asset "${asset_id}" "${tmp_dir}/${archive_name}"; then
    err "failed to download release asset: ${archive_name}"
  fi

  case "$ext" in
    tar.gz)
      require_cmd tar
      tar -xzf "${tmp_dir}/${archive_name}" -C "$tmp_dir"
      ;;
    zip)
      require_cmd unzip
      unzip -q "${tmp_dir}/${archive_name}" -d "$tmp_dir"
      ;;
  esac

  if [ ! -f "${tmp_dir}/${binary_name}" ]; then
    err "archive does not contain expected binary '${binary_name}'"
  fi

  mkdir -p "$install_dir"
  install -m 0755 "${tmp_dir}/${binary_name}" "$install_path"
  note "installed ${app_name} ${version} to ${install_path}"

  case ":${PATH}:" in
    *":${install_dir}:"*) ;;
    *)
      note "warning: ${install_dir} is not on \$PATH; add it to your shell rc to use '${app_name}' directly"
      ;;
  esac

  install_completion "$install_path" "$os"
}

main "$@"
