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
# This script ONLY configures the repo; pkg-install-apt does the apt-get update
# + install. It is called automatically from pkg-install-apt before it installs,
# so both `install.sh` and a hand-run `pkg-install-apt` benefit. Safe to re-run:
# it no-ops when the key and sources list are already in place. Force a refresh
# (e.g. after an upstream key rotation) with GH_REPO_FORCE=1.

set -u

KEYRING=/etc/apt/keyrings/githubcli-archive-keyring.gpg
SOURCES=/etc/apt/sources.list.d/github-cli.list
KEY_URL=https://cli.github.com/packages/githubcli-archive-keyring.gpg

# This repo is a Debian/Ubuntu apt repo; bail cleanly on anything else so the
# caller can fall back to its normal package source.
if ! command -v apt-get >/dev/null 2>&1 || ! command -v dpkg >/dev/null 2>&1; then
  echo "setup_gh_apt_repo: not an apt/dpkg system; skipping." >&2
  exit 0
fi

# Already configured and not forcing a refresh? Nothing to do.
if [ -f "$KEYRING" ] && [ -f "$SOURCES" ] && [ "${GH_REPO_FORCE:-0}" != "1" ]; then
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

echo "setup_gh_apt_repo: done (key: ${KEYRING}, sources: ${SOURCES})."
