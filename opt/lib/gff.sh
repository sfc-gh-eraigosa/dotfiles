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

