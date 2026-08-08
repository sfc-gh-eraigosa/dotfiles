#!/usr/bin/env bash
# Test driver for dir_added_guard.sh — mirrors the assert style of the other
# hook test drivers: every case must pass or the driver exits 1.

set -u
HOOK="$(cd -- "$(dirname "$0")" && pwd -P)/dir_added_guard.sh"
FAILS=0

run_case() {
  desc="$1" input="$2" expect_warn="$3"
  out="$(printf '%s' "$input" | bash "$HOOK")"
  rc=$?
  if [ "$rc" -ne 0 ]; then
    echo "FAIL: $desc — expected exit 0, got $rc"
    FAILS=$((FAILS + 1))
    return
  fi
  if [ "$expect_warn" = "warn" ]; then
    if printf '%s' "$out" | grep -q "additionalContext"; then
      echo "PASS: $desc"
    else
      echo "FAIL: $desc — expected a warning, got: $out"
      FAILS=$((FAILS + 1))
    fi
  else
    if [ -z "$out" ]; then
      echo "PASS: $desc"
    else
      echo "FAIL: $desc — expected silence, got: $out"
      FAILS=$((FAILS + 1))
    fi
  fi
}

run_case "benign project dir is silent" \
  "{\"hook_event_name\":\"DirectoryAdded\",\"directory_path\":\"$HOME/git/some-repo\"}" silent

run_case "~/.ssh warns" \
  "{\"hook_event_name\":\"DirectoryAdded\",\"directory_path\":\"$HOME/.ssh\"}" warn

run_case "subdir of ~/.aws warns" \
  "{\"hook_event_name\":\"DirectoryAdded\",\"directory_path\":\"$HOME/.aws/sso\"}" warn

run_case "tilde form of ~/.config/gss warns" \
  '{"hook_event_name":"DirectoryAdded","directory_path":"~/.config/gss"}' warn

run_case "prefix-similar dir is silent (no false positive on ~/.ssh-backup)" \
  "{\"hook_event_name\":\"DirectoryAdded\",\"directory_path\":\"$HOME/.ssh-backup\"}" silent

run_case "empty input is silent" "" silent

run_case "malformed JSON is silent" "not-json{" silent

if [ "$FAILS" -gt 0 ]; then
  echo "$FAILS case(s) failed"
  exit 1
fi
echo "All dir_added_guard cases passed"
