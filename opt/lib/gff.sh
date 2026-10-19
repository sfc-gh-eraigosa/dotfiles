# shellcheck shell=bash
# gff.sh — fail-open feature-flag gate for install scripts.
# Usage: gff_on <area.component.feature>  (0 = run the step)
# shellcheck disable=SC2154 # _gff_val is assigned by the eval one line above its use
gff_on() {
  _gff_key=$(printf '%s' "$1" | tr '[:lower:]' '[:upper:]' | tr '.-' '__')
  eval "_gff_val=\${GFF_${_gff_key}:-}"
  [ "${_gff_val}" != "false" ]
}
gff_skip_msg() { echo "SKIP (gff: $1=false)"; }

# gff_opt_in - FAIL-CLOSED gate for opt-in steps (boolDefault: false in
# features.yaml): the step runs ONLY when the resolved flag is exactly 'true'
# (via gff set <key> true). Complements the fail-open gff_on above: an opt-in
# step must never run by accident on a machine where gff or the flag export is
# missing, so unset/absent => skip.
# shellcheck disable=SC2154 # _gff_val is assigned by the eval one line above its use
gff_opt_in() {
  _gff_key=$(printf '%s' "$1" | tr '[:lower:]' '[:upper:]' | tr '.-' '__')
  eval "_gff_val=\${GFF_${_gff_key}:-}"
  [ "${_gff_val}" = "true" ]
}

# gff_cli ARGS... - run the gff binary from the repo dir (gff reads the
# checkout's features.yaml relative to cwd). Exit code is passed through.
gff_cli() {
  ( cd "${GFF_REPO_DIR:-.}" || :; "${GFF_BIN:-gff}" "$@" )
}

# gff_user_set KEY - 0 iff this machine set KEY itself, i.e. the winning layer
# in `gff list` (4th column) is user-override or system-override. A missing
# row, a failing gff or any other layer => 1.
gff_user_set() {
  _gus_layer=$(gff_cli list 2>/dev/null | awk -v k="$1" '$1==k {print $4}') || return 1
  case "${_gus_layer}" in
    user-override|system-override) return 0 ;;
    *) return 1 ;;
  esac
}

# gff_require BASE_DIR - ensure a usable gff ("$GFF_BIN" version exits 0).
# Rebuilds via ${GFF_BUILD_CMD:-bash "$BASE_DIR/sdk/gff/build.sh"} when `go` is
# on PATH; a failed/skipped build is a WARNING (stdout) if an existing binary
# still works, else an ERROR (stderr) and return 1.
gff_require() {
  _gr_bin="${GFF_BIN:-$HOME/opt/bin/gff}"
  _gr_why=""
  if command -v go >/dev/null 2>&1; then
    if [ -n "${GFF_BUILD_CMD:-}" ]; then
      sh -c "${GFF_BUILD_CMD}" || _gr_why="build"
    else
      bash "$1/sdk/gff/build.sh" || _gr_why="build"
    fi
  else
    _gr_why="nogo"
  fi
  if "${_gr_bin}" version >/dev/null 2>&1; then
    case "${_gr_why}" in
      build) echo "WARNING: gff build failed; using existing gff" ;;
      nogo) echo "WARNING: no 'go' on PATH; using existing gff" ;;
    esac
    return 0
  fi
  _gr_extra=""
  [ "${_gr_why}" = "nogo" ] && _gr_extra=" (no 'go' on PATH to build it)"
  [ "${_gr_why}" = "build" ] && _gr_extra=" (gff build failed)"
  echo "ERROR: install.sh requires a working gff at ${_gr_bin}${_gr_extra}" >&2
  return 1
}
