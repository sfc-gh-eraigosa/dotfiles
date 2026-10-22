#!/usr/bin/env bash
# =============================================================================
# install_windows.sh — Windows/WSL-only deploy + customization step
#
# Copies opt/Desktop/* onto the real Windows Desktop and runs the PowerShell
# customization chain. The Desktop folder is often OneDrive-redirected, so we
# ask Windows for its actual path via PowerShell and translate it with wslpath.
#
# Usage (called from install.sh):
#   bash "${BASE_DIR}/opt/bin/install_windows.sh" "<BASE_DIR>" [--ask|--deferred|--wsl-network]
#
#   --ask       WSL-detect, check the skip state, prompt y/n/s, persist the
#               choice (~/.cache/dotfiles/win-setup-choice). Runs EARLY in
#               install.sh so all interactivity is front-loaded.
#   --deferred  Consume the recorded choice: deploy Desktop files (always,
#               per-run — independent of the y/n answer), then run the
#               PowerShell customization only for a recorded "y"; record a
#               permanent skip for "s". Runs at the END of install.sh, after
#               the gff bootstrap has exported GFF_* (so the WSLENV hand-off
#               below actually carries the flags).
#   --wsl-network  Apply the recorded WSL networking mode
#               (GFF_INSTALL_WINDOWS_WSL_NETWORK): read-only -Status check, then
#               ONE elevated set-wsl-network.ps1 run only when it would change
#               something. Independent of the desktop-deploy gate.
#   (no mode)   Legacy standalone flow: ask, then execute immediately.
#
# No-op when not running inside WSL / Windows.
# =============================================================================

set -euo pipefail

BASE_DIR="${1:-}"
MODE="${2:---full}"
if [ -z "$BASE_DIR" ]; then
  echo "ERROR: install_windows.sh requires BASE_DIR as the first argument." >&2
  exit 1
fi

# gff gate: source the helper relative to THIS script's location so direct runs
# work too. Stubs mirror each helper's DIRECTION when the lib is absent:
#   gff_on      fail-OPEN  — a missing helper must never block the deploy.
#   gff_opt_in  fail-CLOSED — an opt-in step must never install itself by
#               accident. Without this stub the call would merely be a
#               "command not found" (rc 127) that happens to read as false;
#               state the contract explicitly rather than lean on that.
_gff_lib="$(cd -- "$(dirname "$0")" && pwd -P)/../lib/gff.sh"
if [ -f "$_gff_lib" ]; then
  # shellcheck source=opt/lib/gff.sh
  . "$_gff_lib"
else
  gff_on() { return 0; }
  gff_opt_in() { return 1; }
  gff_skip_msg() { echo "SKIP (gff: $1=false)"; }
fi
# Choice persistence + gff-owned skip state (needs gff_on, so sourced after).
# WINSETUP_REPO_DIR anchors winsetup's gff calls to the checkout: install.sh
# cd's to $HOME late in the run, and gff needs the repo CWD for the repo-live
# layer to resolve install.windows.* keys.
export WINSETUP_REPO_DIR="${BASE_DIR}"
# shellcheck source=opt/lib/winsetup.sh
. "$(cd -- "$(dirname "$0")" && pwd -P)/../lib/winsetup.sh"
# find_powershell: shared PATH-then-System32 lookup (also used by the gsl
# Nerd Font installer).
# shellcheck source=opt/lib/winpowershell.sh
. "$(cd -- "$(dirname "$0")" && pwd -P)/../lib/winpowershell.sh"
# wslnet_detected_mode / wslnet_forget_detected for --wsl-network-auto.
# shellcheck source=opt/lib/wslnet.sh
. "$(cd -- "$(dirname "$0")" && pwd -P)/../lib/wslnet.sh"

# -----------------------------------------------------------------------------
# --wsl-network: apply the WSL networking mode that sshd-setup recorded
# (GFF_INSTALL_WINDOWS_WSL_NETWORK = off|nat|bridged|mirrored). Handled BEFORE
# the desktop-deploy gate and the /proc/version check: the mode is recorded
# independently of the Desktop deploy, and a missing powershell.exe simply
# means there is nothing to apply (exit 0, quiet).
#
# Flow: ask `set-wsl-network.ps1 -Status -Mode <m>` (read-only, no elevation).
#   rc 0 -> already current, done.   rc 3 -> ONE elevated `-Mode <m>` run.
#   rc 4 -> the mode cannot be applied as the machine is (reason printed).
# --wsl-network-auto (what install.sh runs) is the same, except for a mode that
# wslnet_sync recorded because it FOUND it on the machine (wslnet_detected_mode):
# that setup is reported, never changed. Running --wsl-network by hand, or
# choosing another mode, hands it to dotfiles from then on.
# The script dir must be a LOCAL Windows path (an elevated process cannot read a
# \\wsl.localhost UNC path): the Desktop copy when complete, else a copy under
# %LOCALAPPDATA%\dotfiles\wsl-network\. </dev/null on every powershell.exe call
# (it eats the parent's stdin under WSL interop). Never fails the install.
# -----------------------------------------------------------------------------
run_wsl_network_setup() {
  _wn_auto="${1:-}"
  _wn_mode="${GFF_INSTALL_WINDOWS_WSL_NETWORK:-off}"
  case "$_wn_mode" in
    off|"") return 0 ;;
    nat|bridged|mirrored) : ;;
    *) echo "WARNING: unknown WSL network mode '${_wn_mode}' (want nat|bridged|mirrored); skipping." >&2; return 0 ;;
  esac
  # _wn_found: the mode is one this machine already had, so the automatic run
  # only reports it. A marker for another mode is stale: the operator chose since.
  _wn_found=""
  _wn_det="$(wslnet_detected_mode)"
  if [ -n "$_wn_det" ] && [ "$_wn_det" != "$_wn_mode" ]; then
    wslnet_forget_detected
  elif [ -n "$_wn_det" ] && [ "$_wn_auto" = auto ]; then
    _wn_found=1
  fi
  _wn_ps="$(find_powershell || true)"
  [ -n "$_wn_ps" ] || return 0

  _wn_src="${BASE_DIR}/opt/Desktop/Apps/scripts"
  _wn_dir=""
  # Reuse the Desktop path cached by deploy_windows_files when present.
  _wn_desk=""
  if [ -f "${HOME}/.cache/dotfiles/winenv.sh" ]; then
    _wn_desk="$(sed -n 's/^WIN_DESKTOP="\(.*\)"$/\1/p' "${HOME}/.cache/dotfiles/winenv.sh" | head -n 1)"
  fi
  if [ -z "$_wn_desk" ] || [ ! -d "$_wn_desk" ]; then
    _wn_raw="$("$_wn_ps" -NoProfile -Command "[Environment]::GetFolderPath('Desktop')" </dev/null 2>/dev/null | tr -d '\r' || true)"
    _wn_desk="$(wslpath -u "$_wn_raw" 2>/dev/null || true)"
  fi
  # %LOCALAPPDATA% holds the fallback script copy and set-wsl-network.log.
  # shellcheck disable=SC2016  # $env:LOCALAPPDATA is PowerShell syntax
  _wn_raw="$("$_wn_ps" -NoProfile -Command '$env:LOCALAPPDATA' </dev/null 2>/dev/null | tr -d '\r' || true)"
  _wn_la="$(wslpath -u "$_wn_raw" 2>/dev/null || true)"
  if [ -n "$_wn_la" ] && [ ! -d "$_wn_la" ]; then _wn_la=""; fi
  _wn_cand="${_wn_desk}/Apps/scripts"
  if [ -n "$_wn_desk" ] && [ -f "${_wn_cand}/set-wsl-network.ps1" ] \
     && [ -f "${_wn_cand}/refresh-wsl-portproxy.ps1" ] && [ -f "${_wn_cand}/lib/wsl.ps1" ]; then
    _wn_dir="$_wn_cand"
  else
    if [ -z "$_wn_la" ]; then
      echo "WARNING: could not resolve %LOCALAPPDATA%; cannot apply WSL network mode '${_wn_mode}'." >&2
      return 0
    fi
    _wn_dir="${_wn_la}/dotfiles/wsl-network"
    if ! { mkdir -p "${_wn_dir}/lib" \
        && cp -f "${_wn_src}/set-wsl-network.ps1" "${_wn_src}/refresh-wsl-portproxy.ps1" "${_wn_dir}/" \
        && cp -f "${_wn_src}/lib/wsl.ps1" "${_wn_src}/lib/gff.ps1" "${_wn_dir}/lib/"; }; then
      echo "WARNING: could not copy the WSL network scripts to ${_wn_dir}; skipping." >&2
      return 0
    fi
    # Pin the distro (lib\wsl.ps1 reads it) - the elevated child may not see WSLENV.
    if [ -n "${DOTFILES_WSL_DISTRO:-${WSL_DISTRO_NAME:-}}" ]; then
      printf '%s\r\n' "${DOTFILES_WSL_DISTRO:-${WSL_DISTRO_NAME:-}}" > "${_wn_dir}/lib/wsl-distro.txt" \
        || echo "WARNING: could not pin the WSL distro in ${_wn_dir}/lib; the script will auto-detect." >&2
    fi
  fi
  _wn_ps1_w="$(wslpath -w "${_wn_dir}/set-wsl-network.ps1" 2>/dev/null || true)"
  if [ -z "$_wn_ps1_w" ]; then
    echo "WARNING: could not resolve set-wsl-network.ps1 as a Windows path; skipping." >&2
    return 0
  fi

  _wn_log="$(mktemp "${TMPDIR:-/tmp}/wsl-network.XXXXXX" 2>/dev/null || true)"
  if [ -z "$_wn_log" ]; then
    echo "WARNING: could not create a temp file; skipping the WSL network step." >&2
    return 0
  fi
  _wn_rc=0
  "$_wn_ps" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$_wn_ps1_w" -Status -Mode "$_wn_mode" </dev/null >"$_wn_log" 2>&1 || _wn_rc=$?
  tr -d '\r' <"$_wn_log"
  rm -f "$_wn_log"
  case "$_wn_rc" in
    0)
      echo "WSL network '${_wn_mode}' already current"
      # A complete setup run by hand is now dotfiles' to keep current.
      [ -n "$_wn_found" ] || wslnet_forget_detected
      ;;
    3)
      if [ -n "$_wn_found" ]; then
        echo "WSL network '${_wn_mode}' was recorded from this machine's existing setup, so install.sh leaves it as it is (differences above)."
        echo "  To have dotfiles complete and manage it: bash \"${BASE_DIR}/opt/bin/install_windows.sh\" \"${BASE_DIR}\" --wsl-network"
        return 0
      fi
      # The elevation is a UAC prompt on the Windows desktop, and -Wait blocks
      # until someone answers it. With no terminal, or over ssh (a fleet wave
      # uses ssh -t: a terminal, but nobody at that desktop), nobody is there
      # to click, so the install would hang: say what to run instead and leave
      # the machine as it is. WSLNET_TTY is a test seam.
      if [ -n "${SSH_CONNECTION:-}" ] || ! ( : < "${WSLNET_TTY:-/dev/tty}" ) 2>/dev/null; then
        echo "WARNING: WSL network mode '${_wn_mode}' needs an elevated run, and nobody at this machine's desktop can answer UAC (ssh or no terminal); not applying." >&2
        echo "         From an interactive shell: bash \"${BASE_DIR}/opt/bin/install_windows.sh\" \"${BASE_DIR}\" --wsl-network" >&2
        return 0
      fi
      echo "Applying WSL network mode '${_wn_mode}'..."
      echo "  A UAC prompt will appear - approve it (only this step needs admin)."
      # The elevated child's window is separate, so show what it logged: count
      # the log's lines before, print only those appended by this run.
      _wn_applog=""
      [ -n "$_wn_la" ] && _wn_applog="${_wn_la}/dotfiles/set-wsl-network.log"
      _wn_before=0
      if [ -n "$_wn_applog" ] && [ -f "$_wn_applog" ]; then
        _wn_before="$(wc -l <"$_wn_applog" | tr -d ' ')"
      fi
      _wn_psq=$(printf '%s' "$_wn_ps1_w" | sed "s/'/''/g")
      _wn_rc=0
      # -PassThru + exit $p.ExitCode surfaces the elevated child's own exit code.
      # The -File path is wrapped in literal double quotes for paths with spaces.
      "$_wn_ps" -NoProfile -NonInteractive -Command "try { \$p = Start-Process powershell -Verb RunAs -Wait -PassThru -WorkingDirectory \$env:SystemRoot -ArgumentList '-NoProfile','-ExecutionPolicy','Bypass','-File','\"${_wn_psq}\"','-Mode','${_wn_mode}'; exit \$p.ExitCode } catch { Write-Host \$_.Exception.Message; exit 1 }" </dev/null || _wn_rc=$?
      if [ -n "$_wn_applog" ] && [ -f "$_wn_applog" ]; then
        _wn_new="$(tail -n +"$((_wn_before + 1))" "$_wn_applog" | tr -d '\r')"
        if [ -n "$_wn_new" ]; then
          echo "  set-wsl-network.log (this run):"
          printf '%s\n' "$_wn_new" | sed 's/^/    /'
        fi
      fi
      if [ "$_wn_rc" -ne 0 ]; then
        echo "WARNING: elevated set-wsl-network.ps1 exited with code ${_wn_rc} (declined UAC or apply failed)." >&2
        echo "         Re-run: bash \"${BASE_DIR}/opt/bin/install_windows.sh\" \"${BASE_DIR}\" --wsl-network" >&2
      else
        echo "WSL network mode '${_wn_mode}' applied (details: ${_wn_applog:-the set-wsl-network.log under %LOCALAPPDATA%\\dotfiles})."
        wslnet_forget_detected
      fi
      ;;
    4)
      if [ -n "$_wn_found" ]; then
        echo "WSL network '${_wn_mode}' was recorded from this machine's existing setup and is left as it is (see above)."
        return 0
      fi
      echo "WARNING: WSL network mode '${_wn_mode}' cannot be applied on this machine as it is (reason above); nothing changed." >&2
      ;;
    *)
      echo "WARNING: set-wsl-network.ps1 -Status exited with code ${_wn_rc}; not applying." >&2
      ;;
  esac
  return 0
}

if [ "$MODE" = "--wsl-network" ]; then
  run_wsl_network_setup
  exit 0
fi
if [ "$MODE" = "--wsl-network-auto" ]; then
  run_wsl_network_setup auto
  exit 0
fi

gff_on install.windows.desktop-deploy || { gff_skip_msg install.windows.desktop-deploy; exit 0; }

# Per-run marker: set only when the interactive Windows setup actually runs, so
# install.sh can print the Wispr Flow reminder banner at the very end. Cleared on
# every invocation (even non-WSL) so a stale marker never triggers a false banner.
# NOTE: --ask deliberately does NOT clear it — the marker is written by the
# deferred execution later in the same install.sh run and must survive to the
# banner check at the tail.
WIN_SETUP_MARKER="${HOME}/.config/dotfiles/.windows-setup-just-ran"
if [ "$MODE" != "--ask" ]; then
  rm -f "$WIN_SETUP_MARKER" 2>/dev/null || true
fi

# Only run inside WSL (Windows Subsystem for Linux).
if ! grep -qi microsoft /proc/version 2>/dev/null; then
  exit 0
fi

print_prompt_text() {
  echo ""
  echo "Windows Desktop Customization detected."
  echo "Would you like to run the PowerShell setup scripts to configure Windows Terminal,"
  echo "install desktop apps (Discord, Slack, AutoHotkey, etc.), and set up macOS-style hotkeys?"
  echo ""
  echo "Options:"
  echo "  [y] Yes, run setup now."
  echo "  [n] No, skip for now (will ask again next time)."
  echo "  [s] Skip and never ask again (recorded as a gff override)."
  echo ""
}

notty_guidance() {
  # No controlling terminal (CI / fully non-interactive). Windows WAS detected,
  # so don't pretend we asked and don't silently skip — point at the exact
  # command to finish setup interactively.
  echo "No terminal available to prompt; Windows customization not run."
  echo "Windows detected — to configure it, run from an interactive shell:"
  echo "    bash \"${BASE_DIR}/opt/bin/install_windows.sh\" \"${BASE_DIR}\""
}

deploy_windows_files() {
  # -------------------------------------------------------------------------
  # Ensure Windows interop is live. Without the WSLInterop binfmt handler, every
  # Windows .exe (powershell.exe, winget, the wslpath targets below) fails with
  # "exec format error" and this entire Windows setup silently no-ops. WSL normally
  # registers the handler at boot; if it has gone missing (interop disabled, or a
  # flaky restart), re-register it at runtime. binfmt_misc REFUSES a duplicate name,
  # so only register when BOTH known handler names are absent. Session-only; the
  # persistent fix is the wsl-interop-binfmt.service unit installed by
  # opt/scripts/system/wsl_interop_binfmt.sh (called later from install.sh) —
  # WSL's own systemd-binfmt self-heal is condition-blocked under WSL.
  # -------------------------------------------------------------------------
  if [ ! -e /proc/sys/fs/binfmt_misc/WSLInterop ] && [ ! -e /proc/sys/fs/binfmt_misc/WSLInterop-late ]; then
    if [ -e /proc/sys/fs/binfmt_misc/register ]; then
      echo "WSL interop handler not registered; enabling it (may prompt for sudo)..."
      # install.sh caches sudo up front, so this is normally non-interactive.
      sudo bash -c 'echo ":WSLInterop:M::MZ::/init:P" > /proc/sys/fs/binfmt_misc/register' 2>/dev/null \
        && echo "  WSL interop registered." \
        || echo "  WARNING: could not register WSL interop (need root?)." >&2
    else
      echo "WARNING: binfmt_misc not mounted; cannot enable WSL interop." >&2
    fi
  fi

  # -------------------------------------------------------------------------
  # Locate powershell.exe: prefer PATH, fall back to the standard System32 path.
  # (Windows exes are not always on the WSL PATH, e.g. appendWindowsPath=false.)
  # -------------------------------------------------------------------------
  ps_exe="$(find_powershell || true)"

  if [ -z "$ps_exe" ]; then
    echo "NOTE: powershell.exe not found; skipping Windows Desktop deploy."
    exit 0
  fi

  # -------------------------------------------------------------------------
  # Resolve the real Desktop path (may be OneDrive-redirected).
  # -------------------------------------------------------------------------
  # NOTE: </dev/null is load-bearing. powershell.exe (and Windows console exes in
  # general) consume the parent shell's stdin under WSL interop. Without this, the
  # Desktop lookup drains stdin and later interactive reads get EOF.
  win_desktop_raw="$("$ps_exe" -NoProfile -Command "[Environment]::GetFolderPath('Desktop')" </dev/null 2>/dev/null | tr -d '\r')"
  win_desktop="$(wslpath -u "$win_desktop_raw" 2>/dev/null)"

  if [ -z "$win_desktop" ] || [ ! -d "$win_desktop" ]; then
    echo "WARNING: could not resolve Windows Desktop (${win_desktop_raw}); skipping desktop deploy."
    exit 0
  fi

  # -------------------------------------------------------------------------
  # Cache resolved Windows paths for login-time consumers (.bash_aliases).
  # Login fragments must not spawn Windows processes: each costs seconds, and
  # the docker fallback block exists precisely for when interop is broken. So
  # resolve the dynamic values once here — where interop is guaranteed — and
  # cache them; consumers fall back to the standard /mnt/c layout if absent.
  # -------------------------------------------------------------------------
  # shellcheck disable=SC2016  # intentional: $env:ProgramFiles is PowerShell syntax, not shell
  win_program_files_raw="$("$ps_exe" -NoProfile -Command '$env:ProgramFiles' </dev/null 2>/dev/null | tr -d '\r')"
  win_program_files="$(wslpath -u "$win_program_files_raw" 2>/dev/null)"
  winenv_cache_dir="${HOME}/.cache/dotfiles"
  mkdir -p "$winenv_cache_dir"
  {
    echo "# Generated by opt/bin/install_windows.sh — Windows paths in WSL form,"
    echo "# resolved from the real Windows env (\$env:ProgramFiles etc.) via wslpath."
    echo "# Sourced by opt/profiles/.bash_aliases at login; do not edit."
    [ -n "$win_program_files" ] && [ -d "$win_program_files" ] && \
      printf 'WIN_PROGRAM_FILES="%s"\n' "$win_program_files"
    [ -n "$win_desktop" ] && [ -d "$win_desktop" ] && \
      printf 'WIN_DESKTOP="%s"\n' "$win_desktop"
  } > "${winenv_cache_dir}/winenv.sh"
  echo "Cached Windows paths -> ${winenv_cache_dir}/winenv.sh"

  # -------------------------------------------------------------------------
  # Deploy opt/Desktop/* onto the Windows Desktop.
  # -------------------------------------------------------------------------
  echo "Deploying opt/Desktop -> ${win_desktop}"
  # --remove-destination is load-bearing: OneDrive can dehydrate previously
  # deployed files into cloud placeholders (NTFS reparse points) that drvfs
  # cannot open for read or truncate ("Invalid argument" / I/O error on cp).
  # Unlinking first always works, so overwrite via unlink+create. GNU-only
  # flag is fine: this deploy path only runs under WSL (Ubuntu coreutils).
  cp -r --remove-destination "${BASE_DIR}/opt/Desktop/." "${win_desktop}/"

  # Pin the deployed scripts to the distro install.sh runs in: lib\wsl.ps1 reads
  # this marker, so hand runs and scheduled tasks (e.g. RefreshWslPortProxy)
  # target this distro instead of a name hardcoded for another machine.
  # An explicit DOTFILES_WSL_DISTRO wins, as in export_gff_wslenv.
  _wsl_distro="${DOTFILES_WSL_DISTRO:-${WSL_DISTRO_NAME:-}}"
  if [ -n "$_wsl_distro" ]; then
    printf '%s\r\n' "$_wsl_distro" > "${win_desktop}/Apps/scripts/lib/wsl-distro.txt"
    echo "Pinned WSL distro '${_wsl_distro}' -> Apps/scripts/lib/wsl-distro.txt"
  fi
}

# -----------------------------------------------------------------------------
# gff flag pass-through: append every exported GFF_INSTALL_WINDOWS_* name to
# WSLENV (/w = include when invoking Win32 from WSL) BEFORE a powershell.exe
# invocation, so the PowerShell setup scripts' Test-GffOn/Test-GffOptIn gates
# see the same flags. NOTE: the flag was originally /u per the plan — refuted
# empirically 2026-07-26 (P2-T5): /u means Win32->WSL only, so the vars never
# crossed; /w is the WSL->Win32 direction (learn.microsoft.com WSLENV flags).
# Called at DEFERRED time — after install.sh's gff bootstrap `set -a` export —
# and de-duplicates, so repeat calls never grow WSLENV. Unset vars simply never
# appear (fail-open on the Windows side too).
# -----------------------------------------------------------------------------
export_gff_wslenv() {
  _gff_wslenv="${WSLENV:-}"
  # GFF_KEYBOARD_* rides along with GFF_INSTALL_WINDOWS_*: keyboard.macos.enabled
  # is the cross-OS master switch for the macOS key layout, so the Windows side
  # has to see it or the AutoHotkey half would ignore a global opt-out.
  # Two -e expressions rather than a \| alternation: alternation in a BRE is a
  # GNU extension and BSD sed (macOS) does not accept it.
  for _v in $(env | sed -n \
    -e 's/^\(GFF_INSTALL_WINDOWS_[A-Z_]*\)=.*/\1/p' \
    -e 's/^\(GFF_KEYBOARD_[A-Z_]*\)=.*/\1/p'); do
    case ":${_gff_wslenv}:" in *":${_v}/w:"*) : ;; *) _gff_wslenv="${_gff_wslenv:+${_gff_wslenv}:}${_v}/w" ;; esac
  done
  # DOTFILES_WSL_DISTRO rides along too: lib\wsl.ps1 resolves "the one distro"
  # from it first, so a script launched from here targets the distro install.sh
  # runs in. An explicit DOTFILES_WSL_DISTRO from the caller wins.
  _wsl_distro="${DOTFILES_WSL_DISTRO:-${WSL_DISTRO_NAME:-}}"
  if [ -n "$_wsl_distro" ]; then
    export DOTFILES_WSL_DISTRO="$_wsl_distro"
    case ":${_gff_wslenv}:" in *":DOTFILES_WSL_DISTRO/w:"*) : ;; *) _gff_wslenv="${_gff_wslenv:+${_gff_wslenv}:}DOTFILES_WSL_DISTRO/w" ;; esac
  fi
  export WSLENV="${_gff_wslenv}"
}

run_windows_customization() {
  echo "Starting Windows customization... (this may take a few minutes)"

  # gff flag pass-through to the powershell.exe child (see export_gff_wslenv).
  export_gff_wslenv

  # setup-apps.ps1 does the non-elevated app installs, then fires ONE
  # Start-Process -Verb RunAs (setup-elevated.ps1) that performs all admin work
  # in a single elevated child: the macOS-hotkeys logon task, the iTunes Win32
  # MSI, the Wispr Flow MSI, and the PowerToys Copilot-key remap. A single UAC
  # prompt appears during the run — approve it.
  # </dev/null is load-bearing: powershell.exe consumes the parent shell's stdin
  # under WSL interop (see the Desktop-path lookup above for the same guard).
  # NOT bare: under `set -e` a non-zero powershell exit would kill this script
  # BEFORE the log cat below — the exact silent-death mode that hid the first
  # deferred-run failure (2026-07-26). Capture rc, always show the log, warn loud.
  _sa_rc=0
  "$ps_exe" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "${BASE_DIR}/opt/Desktop/Apps/scripts/setup-apps.ps1" </dev/null > /tmp/setup_apps.log 2>&1 || _sa_rc=$?
  cat /tmp/setup_apps.log
  if [ "$_sa_rc" -ne 0 ]; then
    echo "WARNING: setup-apps.ps1 exited with code ${_sa_rc} — see the output above." >&2
    echo "         Re-run just the Windows phase: bash \"${BASE_DIR}/opt/bin/install_windows.sh\" \"${BASE_DIR}\"" >&2
  fi

  # The app/MSI/task elevation all happens inside that single batch. Only Flow's
  # one-time ACCOUNT setup can't be scripted (sign-in, mic, shortcuts off the Win
  # key, start-at-login) — point at the runbook for it.
  wispr_doc_w="$(wslpath -w "${win_desktop}/Apps/scripts/WISPR-FLOW.md" 2>/dev/null)"
  echo ""
  echo "Wispr Flow one-time manual setup (sign-in, mic, shortcuts off Win, start-at-login):"
  echo "    ${wispr_doc_w:-%USERPROFILE%\\Desktop\\Apps\\scripts\\WISPR-FLOW.md}"
  echo ""

  # Mark that the Windows setup ran so install.sh prints the Wispr Flow
  # shortcut reminder banner at the very end (after all other output).
  mkdir -p "$(dirname "$WIN_SETUP_MARKER")"
  : > "$WIN_SETUP_MARKER"
}

# -----------------------------------------------------------------------------
# Opt-in unattended security-audit pipeline (docs/security-audit.md). FAIL-
# CLOSED: unlike every other windows step, this runs ONLY when
# install.windows.security-audit resolves to exactly 'true'
# (gff set install.windows.security-audit true) — an audit installer must never
# appear on a machine by accident, so absent-gff/unset means SKIP (gff_opt_in).
# Independent of the y/n/s customization answer: it rides --deferred right
# after the file deploy, so the deployed Desktop scripts are guaranteed present.
# -----------------------------------------------------------------------------
run_security_audit_setup() {
  if ! gff_opt_in install.windows.security-audit; then
    echo "SKIP (gff: install.windows.security-audit is opt-in and not enabled)"
    return 0
  fi
  if [ -z "${ps_exe:-}" ] || [ -z "${win_desktop:-}" ] || [ ! -d "${win_desktop}" ]; then
    echo "WARNING: Windows paths unresolved; cannot run security-audit setup." >&2
    return 0
  fi
  export_gff_wslenv
  _ssa_ps1_w="$(wslpath -w "${win_desktop}/Apps/scripts/setup-security-audit.ps1" 2>/dev/null || true)"
  if [ -z "${_ssa_ps1_w}" ]; then
    echo "WARNING: could not resolve setup-security-audit.ps1 as a Windows path; skipping." >&2
    return 0
  fi
  echo "Setting up the unattended security-audit pipeline (opt-in flag is ON)..."
  # Same rc-capture pattern as setup-apps.ps1: never die silently under set -e;
  # always surface the log. </dev/null: powershell.exe eats parent stdin (above).
  _ssa_rc=0
  "$ps_exe" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "${_ssa_ps1_w}" </dev/null > /tmp/setup_security_audit.log 2>&1 || _ssa_rc=$?
  cat /tmp/setup_security_audit.log
  if [ "${_ssa_rc}" -ne 0 ]; then
    echo "WARNING: setup-security-audit.ps1 exited with code ${_ssa_rc} — see the output above." >&2
    echo "         Re-run standalone: powershell.exe -ExecutionPolicy Bypass -File \"${_ssa_ps1_w}\"" >&2
  fi
  return 0
}

# -----------------------------------------------------------------------------
# Opt-in Windows security hardening (docs/security-hardening.md). FAIL-CLOSED,
# exactly like run_security_audit_setup above: runs ONLY when
# install.windows.security-hardening resolves to 'true' (gff_opt_in). A step that
# edits group membership, an event channel, and Defender policy must never fire
# by accident, so absent-gff/unset means SKIP.
# The PS script SELF-ELEVATES, so this raises its own UAC prompt — a SECOND one
# when the y/n customization also runs (that chain has its own). It is
# independent of the y/n/s answer and rides --deferred right after the deploy,
# so the deployed Desktop scripts are guaranteed present.
# -----------------------------------------------------------------------------
run_security_hardening_setup() {
  if ! gff_opt_in install.windows.security-hardening; then
    echo "SKIP (gff: install.windows.security-hardening is opt-in and not enabled)"
    return 0
  fi
  if [ -z "${ps_exe:-}" ] || [ -z "${win_desktop:-}" ] || [ ! -d "${win_desktop}" ]; then
    echo "WARNING: Windows paths unresolved; cannot run security-hardening setup." >&2
    return 0
  fi
  export_gff_wslenv
  _ssh_ps1_w="$(wslpath -w "${win_desktop}/Apps/scripts/setup-security-hardening.ps1" 2>/dev/null || true)"
  if [ -z "${_ssh_ps1_w}" ]; then
    echo "WARNING: could not resolve setup-security-hardening.ps1 as a Windows path; skipping." >&2
    return 0
  fi
  echo "Applying opt-in Windows security hardening (opt-in flag is ON)..."
  echo "  A UAC prompt will appear — approve it (the script self-elevates)."
  # Same rc-capture pattern as setup-apps.ps1: never die silently under set -e;
  # always surface the log. </dev/null: powershell.exe eats parent stdin (above).
  _ssh_rc=0
  "$ps_exe" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "${_ssh_ps1_w}" </dev/null > /tmp/setup_security_hardening.log 2>&1 || _ssh_rc=$?
  cat /tmp/setup_security_hardening.log
  if [ "${_ssh_rc}" -ne 0 ]; then
    echo "WARNING: setup-security-hardening.ps1 exited with code ${_ssh_rc} — see the output above." >&2
    echo "         Re-run standalone: powershell.exe -ExecutionPolicy Bypass -File \"${_ssh_ps1_w}\"" >&2
    echo "         Inspect state:     powershell.exe -ExecutionPolicy Bypass -File \"${_ssh_ps1_w}\" -Status" >&2
  fi
  return 0
}

case "$MODE" in
  --ask)
    # Skip state: legacy sentinel (migrated when gff is available) or the gff
    # override install.windows.desktop-deploy=false (env-based via install.sh's
    # early export). Skipped => no prompt, and no choice file for --deferred.
    winsetup_skip_state && exit 0
    print_prompt_text
    choice="$(winsetup_ask)"
    case "$choice" in
      __notty__) notty_guidance ;;
      y)
        winsetup_save_choice y
        echo "Windows customization will run at the END of this install (the UAC prompt appears then)."
        ;;
      s) winsetup_save_choice s ;;
      *) winsetup_save_choice n ;;
    esac
    ;;
  --deferred)
    # Deploy runs per-run regardless of the y/n answer (matches the historical
    # behavior where the file deploy preceded the prompt); the customization
    # and the permanent-skip recording follow the recorded choice.
    deploy_windows_files
    run_security_audit_setup
    run_security_hardening_setup
    case "$(winsetup_take_choice)" in
      y) run_windows_customization ;;
      s) winsetup_record_skip ;;
      *) : ;;   # n, or none (ask never ran/answered): skip customization this run
    esac
    ;;
  --full|*)
    winsetup_skip_state && exit 0
    deploy_windows_files
    run_security_audit_setup
    run_security_hardening_setup
    print_prompt_text
    case "$(winsetup_ask)" in
      __notty__) notty_guidance ;;
      y) run_windows_customization ;;
      s) winsetup_record_skip ;;
      *) echo "Skipping Windows customization for now." ;;
    esac
    ;;
esac
