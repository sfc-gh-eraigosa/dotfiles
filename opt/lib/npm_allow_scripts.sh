# shellcheck shell=bash
# npm_allow_scripts.sh — sourced helper: allow a global npm package's install
# scripts in the user's npm config.
#
# npm >= 11.16 lists every global package whose postinstall is not in the
# `allow-scripts` config ("install scripts not yet covered by allowScripts") on
# each install/update, and a future npm that makes the list strict would stop
# running them — claude-code's postinstall is what installs the claude binary.
# Allowing a package by name (no version) keeps working across its updates.
#
# Usage: npm_allow_scripts <package>
# Never fatal; a no-op without npm or on an npm that predates the key (setting
# an unknown key makes older npm warn instead).

npm_allow_scripts() {
  command -v npm >/dev/null 2>&1 || return 0
  npm config ls -l 2>/dev/null | grep -q '^allow-scripts = ' || return 0
  _nas_list="$(npm config get allow-scripts --location=user 2>/dev/null)"
  case "${_nas_list}" in null | undefined) _nas_list="" ;; esac
  # npm splits the value on commas (@npmcli/config parse-allow-scripts-list.js).
  case ",${_nas_list}," in *",$1,"*) return 0 ;; esac
  npm config set "allow-scripts=${_nas_list:+${_nas_list},}$1" --location=user >/dev/null 2>&1 ||
    echo "WARNING: could not add $1 to npm's allow-scripts"
  return 0
}
