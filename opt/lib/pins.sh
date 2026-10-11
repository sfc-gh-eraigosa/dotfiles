# shellcheck shell=bash
# pins.sh — read tool versions from opt/profiles/versions.tsv (objective `pins`).
#
# Sourced by the installers so their defaults come from one reviewed manifest:
#   . "<repo>/opt/lib/pins.sh"
#   SOPS_VERSION="${SOPS_VERSION:-$(pins_want sops 3.13.1)}"
#
#   pins_manifest              print the manifest path ($PINS_MANIFEST, else
#                              <this lib>/../profiles/versions.tsv)
#   pins_want <tool> [fb]      print the version to install, or `latest`. Always
#                              exits 0; no manifest / no row prints the fallback.
#   pins_latest_gh <o/r>       print the latest GitHub release tag without a v
#                              (honours GITHUB_TOKEN; PINS_GH_API overrides the
#                              API base for tests). Non-zero on failure.
#   pins_strip_v <ver>         drop one leading v from a version (v1.2 -> 1.2)
#
# Host-wide track (gff flag pins.track.mode, exported as GFF_PINS_TRACK_MODE;
# GFF_PINS_TRACK is accepted too and wins):
#   manifest (default) honour each row; latest - every tool follows upstream;
#   pin - rows tracked `latest` use their version column when they have one.
#
# Fail-open and side-effect free: no `set -e`, nothing runs at source time but
# locating this file. Body is POSIX sh (bash 3.2 / dash / zsh safe).

# Locate this file: bash sets BASH_SOURCE; zsh and dash fall back to $0 (zsh's
# $0 is the sourced file; under dash it is the caller, which callers that need
# the default manifest path avoid by setting PINS_MANIFEST/PINS_ROOT).
# shellcheck disable=SC2128 # $BASH_SOURCE (element 0) keeps the line dash-parseable
_pins_self="${BASH_SOURCE:-$0}"
_PINS_LIB_DIR=$(cd -- "$(dirname -- "$_pins_self")" 2>/dev/null && pwd -P)
unset _pins_self

pins_strip_v() {
    case "${1:-}" in
        v[0-9]*) printf '%s\n' "${1#v}" ;;
        *) printf '%s\n' "${1:-}" ;;
    esac
}

pins_manifest() {
    if [ -n "${PINS_MANIFEST:-}" ]; then
        printf '%s\n' "$PINS_MANIFEST"
    else
        printf '%s\n' "${_PINS_LIB_DIR}/../profiles/versions.tsv"
    fi
}

# Repo root that `@file` versions are relative to (PINS_ROOT overrides it).
_pins_root() {
    if [ -n "${PINS_ROOT:-}" ]; then
        printf '%s\n' "$PINS_ROOT"
    else
        printf '%s\n' "${_PINS_LIB_DIR}/../.."
    fi
}

pins_want() {
    _pw_tool="${1:-}"
    _pw_fb="${2:-}"
    _pw_mode="${GFF_PINS_TRACK:-${GFF_PINS_TRACK_MODE:-manifest}}"
    if [ "$_pw_mode" = "latest" ]; then
        printf 'latest\n'
        return 0
    fi
    _pw_file=$(pins_manifest)
    _pw_row=""
    if [ -n "$_pw_tool" ] && [ -r "$_pw_file" ]; then
        _pw_row=$(awk -v t="$_pw_tool" \
            '$1 !~ /^#/ && $1 == t { print $2 " " $3; exit }' "$_pw_file" 2>/dev/null)
    fi
    if [ -z "$_pw_row" ]; then
        pins_strip_v "$_pw_fb"
        return 0
    fi
    _pw_track="${_pw_row%% *}"
    _pw_ver="${_pw_row#* }"
    if [ "$_pw_track" = "latest" ] && [ "$_pw_mode" != "pin" ]; then
        printf 'latest\n'
        return 0
    fi
    case "$_pw_ver" in
        @?*)
            _pw_path="$(_pins_root)/${_pw_ver#@}"
            if [ -r "$_pw_path" ]; then
                _pw_ver=$(tr -d '[:space:]' < "$_pw_path")
            else
                _pw_ver=""
            fi ;;
    esac
    case "$_pw_ver" in
        ""|-|@*) pins_strip_v "$_pw_fb" ;;
        *) pins_strip_v "$_pw_ver" ;;
    esac
    return 0
}

pins_latest_gh() {
    _pl_repo="${1:-}"
    [ -n "$_pl_repo" ] || return 1
    command -v curl >/dev/null 2>&1 || return 1
    _pl_url="${PINS_GH_API:-https://api.github.com}/repos/${_pl_repo}/releases/latest"
    if [ -n "${GITHUB_TOKEN:-}" ]; then
        _pl_json=$(curl -fsSL -H "Accept: application/vnd.github+json" \
            -H "Authorization: Bearer ${GITHUB_TOKEN}" "$_pl_url" 2>/dev/null) || return 1
    else
        _pl_json=$(curl -fsSL -H "Accept: application/vnd.github+json" "$_pl_url" 2>/dev/null) || return 1
    fi
    _pl_tag=$(printf '%s\n' "$_pl_json" \
        | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
    [ -n "$_pl_tag" ] || return 1
    pins_strip_v "$_pl_tag"
}
