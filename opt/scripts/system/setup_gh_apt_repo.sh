#!/usr/bin/env bash
#
# setup_gh_apt_repo.sh — register GitHub CLI's official apt repository + signing
# key so `apt install gh` pulls the latest release instead of the stale version
# in Ubuntu's `universe` (which lags many minor releases behind).
#
# Why this exists:
#   * gh is listed once in opt/profiles/packages.tsv and installed by
#     pkg-install-apt. Out of the box apt resolves it from `universe`, which can
#     be 40+ releases behind (e.g. 2.46 vs upstream ~2.92).
#   * macOS gets a current gh from Homebrew, so this gap is apt-only.
#   * Adding the official repo here means the existing manifest-driven
#     `apt install gh` resolves to — and upgrades to — the latest on every run.
#
# Adding the repo is NOT sufficient on its own (issue #255):
#   Ubuntu Pro's ESM ships /etc/apt/preferences.d/ubuntu-pro-esm-apps, which pins
#   EVERY package from `o=UbuntuESMApps` to priority 510 — above the default 500
#   that this repo gets. So apt kept gh 2.45.0 (Feb 2024) from ESM even with the
#   official repo present and offering 2.98.0:
#
#     2.98.0                 500  https://cli.github.com/packages
#     2.45.0-...+esm3        510  https://esm.ubuntu.com/apps/ubuntu   <-- won
#
#   ESM backports security fixes but not API-compatibility changes, so this does
#   not heal itself: gh 2.45.0 still requests the `projectCards` GraphQL field
#   that GitHub sunset with Projects (classic), which broke every
#   `gh pr edit --body` and `--add-label` — and, downstream, `gss feature
#   checkpoint`'s PR-body refresh. Hence the PREFERENCES pin below.
#
# This script ONLY configures the repo; pkg-install-apt does the apt-get update
# + install. It is called automatically from pkg-install-apt before it installs,
# so both `install.sh` and a hand-run `pkg-install-apt` benefit. Safe to re-run:
# it no-ops when the key and sources list are already in place. Force a refresh
# (e.g. after an upstream key rotation) with GH_REPO_FORCE=1.

set -u

KEYRING=/etc/apt/keyrings/githubcli-archive-keyring.gpg
SOURCES=/etc/apt/sources.list.d/github-cli.list
PREFS=/etc/apt/preferences.d/github-cli
KEY_URL=https://cli.github.com/packages/githubcli-archive-keyring.gpg

# write_prefs — make the official repo outrank Ubuntu Pro ESM (510) and the
# archive (500) for gh specifically. Scoped to `Package: gh` so this never
# affects any other package's resolution.
#
# Run OUTSIDE the "already configured" early-exit below: hosts provisioned
# before this pin existed have the key and sources but no preferences file, and
# they are exactly the ones stuck on the stale gh. Re-running must heal them.
write_prefs() {
  if [ -f "$PREFS" ] && [ "${GH_REPO_FORCE:-0}" != "1" ] &&
     grep -q 'cli\.github\.com' "$PREFS" 2>/dev/null; then
    return 0
  fi
  echo "setup_gh_apt_repo: pinning gh to the official repo (above Ubuntu Pro ESM)..."
  sudo mkdir -p -m 755 /etc/apt/preferences.d || return 1
  # A heredoc through `sudo tee` keeps this a single privileged write.
  cat <<'PREF_EOF' | sudo tee "$PREFS" >/dev/null || return 1
# Managed by dotfiles: opt/scripts/system/setup_gh_apt_repo.sh
#
# Ubuntu Pro ESM pins every package it carries to 510 (see
# /etc/apt/preferences.d/ubuntu-pro-esm-apps), which outranks the default 500
# this repo would otherwise get and holds gh at a stale release. 600 puts the
# official GitHub CLI repo on top for gh, and only for gh.
Package: gh
Pin: origin cli.github.com
Pin-Priority: 600
PREF_EOF
  return 0
}

# This repo is a Debian/Ubuntu apt repo; bail cleanly on anything else so the
# caller can fall back to its normal package source.
if ! command -v apt-get >/dev/null 2>&1 || ! command -v dpkg >/dev/null 2>&1; then
  echo "setup_gh_apt_repo: not an apt/dpkg system; skipping." >&2
  exit 0
fi

# Already configured and not forcing a refresh? Still ensure the pin exists —
# it postdates the original script, so an existing install can have the repo
# without it (and is therefore still stuck on the ESM version).
if [ -f "$KEYRING" ] && [ -f "$SOURCES" ] && [ "${GH_REPO_FORCE:-0}" != "1" ]; then
  write_prefs || echo "setup_gh_apt_repo: WARNING: could not write $PREFS." >&2
  echo "setup_gh_apt_repo: GitHub CLI apt repo already configured; skipping (GH_REPO_FORCE=1 to refresh)."
  exit 0
fi

# Pick whatever fetcher is present — curl ships on virtually every base image
# and is already used by install.sh; wget is the upstream-documented tool.
if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -nv -O "$2" "$1"; }
else
  echo "setup_gh_apt_repo: need curl or wget to fetch the signing key; skipping." >&2
  exit 1
fi

echo "setup_gh_apt_repo: configuring GitHub CLI apt repository for latest gh..."

# Download the key to a temp file first so only the install needs sudo, then
# place it with world-readable perms apt requires for a signed-by keyring.
tmpkey="$(mktemp)"
trap 'rm -f "$tmpkey"' EXIT
if ! fetch "$KEY_URL" "$tmpkey"; then
  echo "setup_gh_apt_repo: failed to download signing key from $KEY_URL." >&2
  exit 1
fi

sudo mkdir -p -m 755 /etc/apt/keyrings || exit 1
sudo install -m 0644 "$tmpkey" "$KEYRING" || exit 1

# arch=... + signed-by=... scopes the repo to this machine's architecture and
# pins it to the key we just installed.
arch="$(dpkg --print-architecture)"
echo "deb [arch=${arch} signed-by=${KEYRING}] https://cli.github.com/packages stable main" \
  | sudo tee "$SOURCES" >/dev/null || exit 1

write_prefs || echo "setup_gh_apt_repo: WARNING: could not write $PREFS." >&2

echo "setup_gh_apt_repo: done (key: ${KEYRING}, sources: ${SOURCES}, prefs: ${PREFS})."
