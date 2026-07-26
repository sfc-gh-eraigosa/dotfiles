#!/usr/bin/env bash
#
# shell-portability-scan.sh — cross-shell / cross-OS portability scanner.
#
# Catches the class of portability bugs that `shellcheck` and `bash -n` MISS,
# per docs/mbo/specs/shell-portability.md. The motivating incident: a bash-only
# `function name { ... }` block in a profile fragment, sourced by dash at GUI
# login on a Raspberry Pi, fatally aborted the LightDM Xsession (dash exits 2 on
# a parse error in a dot-sourced file, even under `set +e`) → blank screen →
# login loop. `bash -n` passes such files; only `dash -n` catches them.
#
# Three tiers of findings:
#   TIER 1  MUST-FIX  — files that MUST be POSIX (a `#!/bin/sh` shebang, or a
#                       fragment that dash sources at login: .profile,
#                       .xsessionrc) that FAIL `dash -n`. These are the
#                       login-breaking class.
#   TIER 2  macOS     — bash-4-only features (declare -A, ${v,,}, mapfile, |&)
#           HAZARDS     and BSD-vs-GNU coreutil traps (sed -i, stat -c,
#                       readlink -f, grep -P, base64 -w, `which`, date -d). These
#                       break on macOS (BSD coreutils + bash 3.2).
#   TIER 3  INFO      — every shell file that is not POSIX-clean (`dash -n`),
#           ("everything") regardless of shebang. Bash scripts are ALLOWED to use
#                       bashisms, so most entries here are expected; the list is
#                       the full "which scripts are not dash-portable" picture.
#
# `--strict` makes a non-empty TIER 1 or TIER 2 exit non-zero — this is the
# enforcing CI gate (`make lint-portability`). Without it the scan is advisory
# and always exits 0. To intentionally exempt a reviewed line, add a trailing
# `# portability-ok: <reason>` comment and the scanner will skip it.
# `--md` emits a GitHub-flavoured-markdown report (used to seed the issue body).
#
# Usage:
#   opt/scripts/system/shell-portability-scan.sh [--strict] [--md]
#
# Portability: this script targets bash 3.2+ (no associative arrays, no
# mapfile) so it runs identically on macOS system bash and CI ubuntu bash.

set -u

STRICT=0
MD=0
for arg in "$@"; do
  case "$arg" in
    --strict) STRICT=1 ;;
    --md)     MD=1 ;;
    -h|--help)
      sed -n '2,40p' "$0"; exit 0 ;;
    *) echo "unknown arg: $arg" >&2; exit 2 ;;
  esac
done

# Resolve repo root from this script's location (works in a worktree too).
unset CDPATH
SCRIPT_DIR=$(cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(cd -- "${SCRIPT_DIR}/../../.." && pwd)
cd "$REPO_ROOT" || exit 2

if ! command -v dash >/dev/null 2>&1; then
  echo "WARNING: dash not installed; cannot run the POSIX syntax check. Install with 'apt-get install dash'." >&2
  # Not fatal in warn mode — emit an empty-ish report and exit 0.
  [ "$STRICT" -eq 1 ] && exit 0
fi

# ---------------------------------------------------------------------------
# 1. Enumerate candidate shell files (tracked only), excluding vendored trees.
# ---------------------------------------------------------------------------
# A file is a candidate if it ends in .sh OR its first line is a shell shebang
# OR it is one of the known sourced profile fragments.
FRAGMENTS="opt/profiles/.profile opt/profiles/.bashrc opt/profiles/.bash_aliases opt/profiles/.bash_logout opt/profiles/.docker.sh opt/profiles/.goenv.sh opt/profiles/.nano_profile opt/profiles/.xsessionrc"

# Of those, only these are dot-sourced by dash (/bin/sh) at login — directly
# (the Xsession sources .profile and .xsessionrc) or transitively (.profile
# unconditionally dot-sources .nano_profile). THESE must be POSIX-clean. The
# rest (.bashrc, .bash_aliases, .docker.sh, .goenv.sh, …) are sourced only by
# bash/zsh, so bashisms in them are legitimate and are NOT a must-fix.
POSIX_FRAGMENTS="opt/profiles/.profile opt/profiles/.xsessionrc opt/profiles/.nano_profile"

is_excluded() {
  case "$1" in
    */opt/google-cloud-sdk/*|opt/google-cloud-sdk/*) return 0 ;;
    archive/*)                                        return 0 ;;
    *) return 1 ;;
  esac
}

# zsh files must NOT be dash/bash-checked (legitimate zsh syntax).
is_zsh() {
  case "$1" in
    *.zshrc|*.p10k.zsh|*.zshenv|*.zprofile) return 0 ;;
  esac
  head -1 "$1" 2>/dev/null | grep -qE '^#!.*zsh' && return 0
  return 1
}

# A file is POSIX-REQUIRED if dash sources/executes it: a /bin/sh shebang, or a
# fragment dash sources at login.
is_posix_required() {
  case " $POSIX_FRAGMENTS " in
    *" $1 "*) return 0 ;;     # .profile / .xsessionrc / .nano_profile (dash-sourced)
  esac
  head -1 "$1" 2>/dev/null | grep -qE '^#! ?(/usr/bin/env +sh|/bin/sh)\>' && return 0
  return 1
}

CANDIDATES=$(
  {
    git ls-files '*.sh'
    for f in $FRAGMENTS; do [ -f "$f" ] && echo "$f"; done
    # shebang-based discovery for extension-less scripts
    git ls-files | while IFS= read -r f; do
      [ -f "$f" ] || continue
      head -1 "$f" 2>/dev/null | grep -qE '^#!.*(bash|/sh|zsh)\>' && echo "$f"
    done
  } | sort -u
)

# ---------------------------------------------------------------------------
# 2. Run the three tiers.
# ---------------------------------------------------------------------------
TIER1=""   # must-fix: posix-required files failing dash -n  (file\treason)
TIER3=""   # info: any file failing dash -n
TIER2=""   # macOS hazards: file:line:pattern\tdescription

# macOS hazard patterns: "ERE<TAB>description". Heuristic (warn-only).
HAZARDS='declare[[:space:]]+-A	bash-4 associative array (breaks macOS bash 3.2)
\$\{[A-Za-z_][A-Za-z0-9_]*(\^\^|,,|\^|,)[}/:]	bash-4 case-conversion ${v,,}/${v^^} (breaks macOS bash 3.2)
\b(mapfile|readarray)\b	bash-4 mapfile/readarray (breaks macOS bash 3.2)
\|&([^]]|$)	bash-4 |& pipe (breaks macOS bash 3.2)
\bsed[[:space:]]+-i[[:space:]]	sed -i with a space arg differs GNU vs BSD; use the portable `sed -i.bak … && rm -f …bak` (suffix attached) or a tmpfile+mv
\bstat[[:space:]]+-c\b	stat -c is GNU; BSD uses `stat -f`
\breadlink[[:space:]]+-f\b	readlink -f is GNU; old BSD lacks it; ship a realpath shim
\bgrep[[:space:]]+-[A-Za-z]*P\b	grep -P (PCRE) is GNU-only; use -E (ERE)
\bbase64[[:space:]]+-w\b	base64 -w is GNU-only; use `base64 | tr -d "\\n"`
\bdate[[:space:]]+-d\b	date -d is GNU; BSD uses `date -r`/`-v`
(^|[;&|]|\$\()[[:space:]]*which[[:space:]]	use `command -v`, not `which` (not guaranteed installed; BSD/GNU differ)
\bread[[:space:]]+-[A-Za-z]*A\b	zsh-only `read -A`; bash errors. Use `read -a`/mapfile
(^|[;&|])[[:space:]]*(sudo[[:space:]]+)?apt(-get)?[[:space:]]+install\b	apt install without DEBIAN_FRONTEND=noninteractive — a debconf prompt (tzdata-class) hangs non-tty contexts like docker build/CI (the PR #182 hang); use `sudo DEBIAN_FRONTEND=noninteractive apt-get install …`'

scan_count=0
for f in $CANDIDATES; do
  is_excluded "$f" && continue
  scan_count=$((scan_count + 1))

  # --- dash -n (skip zsh files) ---
  if ! is_zsh "$f"; then
    if command -v dash >/dev/null 2>&1; then
      err=$(dash -n "$f" 2>&1)
      if [ -n "$err" ]; then
        reason=$(echo "$err" | head -1 | sed "s#^$f: *##")
        TIER3="${TIER3}${f}	${reason}
"
        if is_posix_required "$f"; then
          TIER1="${TIER1}${f}	${reason}
"
        fi
      fi
    fi
  fi

  # --- macOS hazard heuristics (skip zsh files; read -A check also skips .zshrc handled above) ---
  is_zsh "$f" && continue
  # The scanner's own HAZARDS table necessarily contains every pattern it greps
  # for, so don't let it flag itself. (dash -n / Tier 1 / Tier 3 still apply.)
  case "$f" in */shell-portability-scan.sh) continue ;; esac
  while IFS=$(printf '\t') read -r pat desc; do
    [ -n "$pat" ] || continue
    # Suppress the stat -c hazard when the file already has a BSD `stat -f`
    # probe+fallback — the documented portable idiom (spec §4).
    case "$desc" in
      *"stat -c"*) grep -q 'stat -f' "$f" && continue ;;
    esac
    # Drop comment-only matches (first non-space char is #) and any line that
    # carries a `# portability-ok` opt-out (reviewed/intentional exceptions;
    # see docs/mbo/specs/shell-portability.md + CLAUDE.md).
    hits=$(grep -nE "$pat" "$f" 2>/dev/null | grep -vE '^[0-9]+:[[:space:]]*#' | grep -vF 'portability-ok' || true)
    # The apt rule only fires when the invocation line lacks the guard.
    case "$desc" in
      *"DEBIAN_FRONTEND"*) hits=$(printf '%s\n' "$hits" | grep -vF 'DEBIAN_FRONTEND' || true) ;;
    esac
    if [ -n "$hits" ]; then
      while IFS= read -r h; do
        [ -n "$h" ] || continue
        TIER2="${TIER2}${f}:${h%%:*}	${desc}
"
      done <<EOF
$hits
EOF
    fi
  done <<EOF
$HAZARDS
EOF
done

# ---------------------------------------------------------------------------
# 3. Report.
# ---------------------------------------------------------------------------
n1=$(printf '%s' "$TIER1" | grep -c . || true)
n2=$(printf '%s' "$TIER2" | grep -c . || true)
n3=$(printf '%s' "$TIER3" | grep -c . || true)

if [ "$MD" -eq 1 ]; then
  printf '## Shell portability scan\n\n'
  printf '_Generated by `opt/scripts/system/shell-portability-scan.sh`. Targets: Raspberry Pi / WSL (dash `/bin/sh`), macOS (BSD coreutils + bash 3.2), Linux._\n\n'
  printf 'Scanned **%s** shell files.\n\n' "$scan_count"
  printf '### Tier 1 — MUST FIX (breaks POSIX `/bin/sh`; the login-loop class) — %s\n\n' "$n1"
  if [ "$n1" -eq 0 ]; then printf '_None — all POSIX-required files pass `dash -n`._\n\n'; else
    printf '| File | `dash -n` error |\n|---|---|\n'
    printf '%s' "$TIER1" | while IFS=$(printf '\t') read -r f r; do [ -n "$f" ] && printf '| `%s` | %s |\n' "$f" "$r"; done
    printf '\n'
  fi
  printf '### Tier 2 — macOS hazards (BSD coreutils / bash 3.2) — %s\n\n' "$n2"
  if [ "$n2" -eq 0 ]; then printf '_None detected._\n\n'; else
    printf '| File:line | Hazard |\n|---|---|\n'
    printf '%s' "$TIER2" | while IFS=$(printf '\t') read -r fl d; do [ -n "$fl" ] && printf '| `%s` | %s |\n' "$fl" "$d"; done
    printf '\n'
  fi
  printf '### Tier 3 — informational: all files not POSIX-clean (`dash -n`) — %s\n\n' "$n3"
  printf '<details><summary>Expand (%s files — bash scripts are allowed to use bashisms)</summary>\n\n' "$n3"
  if [ "$n3" -eq 0 ]; then printf '_None._\n'; else
    printf '%s' "$TIER3" | while IFS=$(printf '\t') read -r f r; do [ -n "$f" ] && printf -- '- `%s` — %s\n' "$f" "$r"; done
  fi
  printf '\n</details>\n'
else
  echo "==> shell-portability-scan  (scanned $scan_count files)"
  echo
  echo "TIER 1 — MUST FIX (POSIX /bin/sh breakage; login-loop class): $n1"
  if [ "$n1" -gt 0 ]; then printf '%s' "$TIER1" | while IFS=$(printf '\t') read -r f r; do [ -n "$f" ] && echo "  ✗ $f — $r"; done; fi
  echo
  echo "TIER 2 — environment hazards (macOS bash 3.2 / BSD coreutils / non-interactive apt): $n2"
  if [ "$n2" -gt 0 ]; then printf '%s' "$TIER2" | while IFS=$(printf '\t') read -r fl d; do [ -n "$fl" ] && echo "  ⚠ $fl — $d"; done; fi
  echo
  echo "TIER 3 — informational (all non-POSIX-clean files): $n3 (run with --md for the full list)"
  echo
  if [ "$n1" -gt 0 ]; then
    echo "::warning::$n1 POSIX-required shell file(s) fail 'dash -n' — see docs/mbo/specs/shell-portability.md"
  fi
fi

# In --strict mode a non-empty Tier 1 OR Tier 2 fails the gate; Tier 3 is
# informational and never gates. Without --strict the scan is purely advisory
# (always exits 0).
if [ "$STRICT" -eq 1 ] && { [ "$n1" -gt 0 ] || [ "$n2" -gt 0 ]; }; then
  exit 1
fi
exit 0
