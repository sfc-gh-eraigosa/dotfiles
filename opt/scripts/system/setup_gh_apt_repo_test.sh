#!/usr/bin/env bash
# Test driver for opt/scripts/system/setup_gh_apt_repo.sh
#
# The script's whole purpose is "gh installs the LATEST upstream release".
# Registering the apt repo turns out not to achieve that on an Ubuntu Pro host
# (issue #255): ESM ships a `Package: *` pin at priority 510, above the 500 the
# official repo gets, so apt held gh at 2.45.0 (Feb 2024) — old enough that its
# GraphQL queries still ask for the `projectCards` field GitHub sunset, which
# broke `gh pr edit` and `gss feature checkpoint`'s PR-body refresh.
#
# These cases lock in the pin that fixes it, and — importantly — that a host
# provisioned BEFORE the pin existed gets healed on the next run rather than
# skipped by the "already configured" early-exit.
#
# The script needs sudo and network, so we do not execute it. We assert on its
# source: the pin's content, its scope, and its placement relative to the
# early-exit. That is the part that was wrong.
#
# Run: bash opt/scripts/system/setup_gh_apt_repo_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/setup_gh_apt_repo.sh"

assert_file_exists "${SCRIPT}" "setup_gh_apt_repo.sh exists"
assert_exit_code 0 "parses with bash -n" bash -n "${SCRIPT}"

# --- the pin itself -------------------------------------------------------
assert_grep "writes an apt preferences file" \
    'PREFS=/etc/apt/preferences.d/github-cli' "${SCRIPT}"
assert_grep "pins by origin cli.github.com" \
    'Pin: origin cli\.github\.com' "${SCRIPT}"

# 600 must beat ESM's 510. A lower value silently reintroduces the bug, so
# assert the exact number rather than merely that a priority exists.
assert_grep "pin priority is 600 (> ESM's 510)" \
    'Pin-Priority: 600' "${SCRIPT}"

# Scoped to gh ONLY. `Package: *` here would let the GitHub repo outrank ESM
# for anything it happens to ship — a much wider blast radius than intended.
# Anchored: the bare substring would also match a hypothetical `Package: ghci`,
# so the assertion looked stronger than it was. assert_grep is grep -E.
assert_grep "pin is scoped to gh, not Package: *" \
    'Package: gh[[:space:]]*$' "${SCRIPT}"
assert_grep_negative "pin does NOT use a wildcard package" \
    'Package: \*' "${SCRIPT}"

# --- healing existing hosts ----------------------------------------------
# The regression that made this necessary: the early-exit returns before any
# pin work when the key+sources already exist. Hosts provisioned before the pin
# are exactly the stuck ones, so write_prefs must be called on that path too.
assert_grep "write_prefs is defined" '^write_prefs\(\)' "${SCRIPT}"

early_exit_line=$(grep -n 'GitHub CLI apt repo already configured' "${SCRIPT}" | head -1 | cut -d: -f1)
prefs_before_exit=$(awk -v stop="${early_exit_line}" \
    'NR < stop && /write_prefs \|\|/ {found=1} END {print found+0}' "${SCRIPT}")
assert_eq "${prefs_before_exit}" "1" \
    "write_prefs runs BEFORE the already-configured early-exit (heals old hosts)"

# And on the fresh-install path as well.
assert_grep "prefs path is reported in the completion message" \
    'prefs: \$\{PREFS\}' "${SCRIPT}"

# --- documentation ties the fix to its cause ------------------------------
# One assertion, keyed on the NUMBER rather than the prose. The previous pair
# matched 'UbuntuESMApps' and 'projectCards' in the header comment, which made
# a reword fail the suite while the fix was intact — a test spending its
# failure budget on wording. 510 is the ESM priority the pin must beat: a fact
# that cannot be edited away without the explanation becoming wrong.
assert_grep "header documents the ESM priority (510) this pin must beat" \
    '510' "${SCRIPT}"

# A pin-write failure must never exit 0. Both call sites once warned and
# continued, so pkg-install-apt saw success and installed the stale ESM build
# on a host that then read as healed. Guard the fix, not just its presence.
assert_grep_negative "no write_prefs call soft-fails to a bare warning" \
    'write_prefs \|\| echo' "${SCRIPT}"
# BOTH call sites — the heal path and the fresh-install path — must fail hard,
# so count the failure message rather than trusting one of them.
assert_eq "$(grep -c 'gh would stay pinned to ESM' "${SCRIPT}")" "2" \
    "both write_prefs call sites exit non-zero on a failed pin write"

# --- safety ---------------------------------------------------------------
# Still a no-op off apt systems (macOS gets gh from Homebrew).
assert_grep "bails cleanly on non-apt systems" \
    'not an apt/dpkg system; skipping' "${SCRIPT}"

# The preferences directory must exist before the write, or tee fails on a
# minimal image.
assert_grep "creates /etc/apt/preferences.d before writing" \
    'mkdir -p -m 755 /etc/apt/preferences\.d' "${SCRIPT}"

_test_report
