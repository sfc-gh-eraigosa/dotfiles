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
