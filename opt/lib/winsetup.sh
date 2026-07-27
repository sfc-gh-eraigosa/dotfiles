# shellcheck shell=sh
# winsetup.sh — Windows-setup choice + skip-state helpers (POSIX/dash-safe).
# Sourced by opt/bin/install_windows.sh AFTER opt/lib/gff.sh (needs gff_on).
# Test seams (override via env): WINSETUP_CHOICE_FILE, WINSETUP_SENTINEL,
# WINSETUP_GFF. All functions are silent on success unless they change state.

winsetup_choice_file() { printf '%s\n' "${WINSETUP_CHOICE_FILE:-${HOME}/.cache/dotfiles/win-setup-choice}"; }
winsetup_sentinel()    { printf '%s\n' "${WINSETUP_SENTINEL:-${HOME}/.config/dotfiles/.skip_windows_setup}"; }
winsetup_gff()         { printf '%s\n' "${WINSETUP_GFF:-gff}"; }

# Run gff from WINSETUP_REPO_DIR (the dotfiles checkout) in a subshell so the
# repo-live layer resolves the install.windows.* keys regardless of the
# caller's CWD — install.sh cd's to $HOME late in the run, which made a
# deferred `gff set` fail with "unknown flag key" (live regression 2026-07-26).
# Mirrors install.sh's own bootstrap: eval "$(cd "$BASE_DIR" && gff export …)".
winsetup_run_gff() {
  ( cd "${WINSETUP_REPO_DIR:-.}" 2>/dev/null || :; "$(winsetup_gff)" "$@" )
}

# 0 = Windows setup is permanently skipped. Single source of truth is the gff
# override install.windows.desktop-deploy=false; a legacy sentinel file is
# migrated to it when a working gff exists, and honored as-is when not.
winsetup_skip_state() {
  _ws_sent="$(winsetup_sentinel)"
  if [ -f "$_ws_sent" ]; then
    if command -v "$(winsetup_gff)" >/dev/null 2>&1 \
       && winsetup_run_gff set install.windows.desktop-deploy false >/dev/null 2>&1; then
      rm -f "$_ws_sent"
      echo "Migrated legacy skip sentinel to gff override: install.windows.desktop-deploy=false"
    fi
    return 0
  fi
  if ! gff_on install.windows.desktop-deploy; then
    return 0
  fi
  return 1
}

# Record the "[s] never ask again" choice: gff override first, sentinel fallback.
winsetup_record_skip() {
  if command -v "$(winsetup_gff)" >/dev/null 2>&1 \
     && winsetup_run_gff set install.windows.desktop-deploy false; then
    echo "Recorded permanent skip as a gff override (undo: gff unset install.windows.desktop-deploy)."
  else
    _ws_sent="$(winsetup_sentinel)"
    mkdir -p "$(dirname "$_ws_sent")"
    echo "User chose to skip Windows setup on $(date)" > "$_ws_sent"
    echo "gff unavailable; wrote legacy sentinel ${_ws_sent}"
  fi
}

# Prompt on the controlling terminal; echoes y|n|s|__notty__. The caller prints
# the option text — this only reads. Anything unrecognized maps to n (skip once).
# NOTE: probe by actually opening /dev/tty — `[ -r /dev/tty ]` is a false
# positive in sessions with no controlling terminal (access(2) checks the
# inode's mode bits, not ctty presence; verified 2026-07-26).
winsetup_ask() {
  if ( : < /dev/tty ) 2>/dev/null; then
    printf 'Choice [y/n/s]: ' > /dev/tty
    read -r _ws_choice < /dev/tty || _ws_choice=""
    case "$_ws_choice" in
      y|Y) echo y ;;
      s|S) echo s ;;
      *)   echo n ;;
    esac
  else
    echo __notty__
  fi
}

winsetup_save_choice() {  # $1 = y|n|s
  _ws_cf="$(winsetup_choice_file)"
  mkdir -p "$(dirname "$_ws_cf")"
  printf '%s\n' "$1" > "$_ws_cf"
}

winsetup_take_choice() {  # echoes the recorded choice or "none"; consumes the file
  _ws_cf="$(winsetup_choice_file)"
  if [ -f "$_ws_cf" ]; then
    cat "$_ws_cf"
    rm -f "$_ws_cf"
  else
    echo none
  fi
}
