#!/usr/bin/env bash
#
# setup_git_apt_repo.sh — register the git-core PPA (Ubuntu Git Maintainers) +
# its signing key so `apt install git` resolves to a CURRENT git on Ubuntu
# hosts instead of the release the distro froze at.
#
# Why this exists (dotfiles#343):
#   * Ubuntu 22.04 ships git 2.34.1 (Nov 2021) and never moves it. claude's
#     plugin updater clones a marketplace with `--depth 1 --filter=tree:0`, sets
#     a sparse cone, then runs `git checkout <commit>` — and git 2.34.1
#     SEGFAULTS on that checkout (exit 139). A crashed git cannot remove its
#     .git/index.lock, so the next checkout dies on "index.lock: File exists",
#     which is the failure every `fleet update` has logged on the Jetson.
#     Found with a PATH shim logging each git exit status; git 2.55.0 from this
#     PPA, dropped in without touching the system, ran the same four updates
#     cleanly on that node (2026-09-19). A retry can never fix it: each attempt
#     is a fresh clone that crashes the same way.
#   * git is listed in opt/profiles/packages.tsv and installed by
#     pkg-install-apt; with this repo registered the SAME manifest-driven
#     `apt install git` resolves to — and upgrades to — the PPA's build.
#   * macOS gets a current git from Homebrew; Debian has no builds in this PPA,
#     so it is Ubuntu-only and skips elsewhere.
#
# Adding the repo is not sufficient on its own: Ubuntu Pro's ESM pins every
# package it ships at priority 510, above the 500 a third-party repo gets
# (issue #255, the same trap the gh repo hit). Hence the PREFERENCES pin below.
#
# This script ONLY configures the repo; pkg-install-apt does the apt-get update
# + install. Safe to re-run: it refreshes the key and pin, and no-ops the rest
# when already in place. Force a full rewrite with GIT_REPO_FORCE=1.
set -u

KEYRING=/etc/apt/keyrings/git-core-ppa-keyring.gpg
SOURCES=/etc/apt/sources.list.d/git-core-ppa.list
PREFS=/etc/apt/preferences.d/git-core-ppa
# The PPA's signing key, by fingerprint (Launchpad: ~git-core/+archive/ubuntu/ppa).
FINGERPRINT=F911AB184317630C59970973E363C90F8F1B6217
KEY_URL="https://keyserver.ubuntu.com/pks/lookup?op=get&search=0x${FINGERPRINT}"
REPO_URL=https://ppa.launchpadcontent.net/git-core/ppa/ubuntu

if ! command -v apt-get >/dev/null 2>&1 || ! command -v dpkg >/dev/null 2>&1; then
  echo "setup_git_apt_repo: not an apt/dpkg system; skipping." >&2
  exit 0
fi
# Ubuntu only: the PPA publishes no Debian builds, and a Debian host pointed at
# it would just get apt errors on every update.
OS_ID=""; CODENAME=""
if [ -r /etc/os-release ]; then
  OS_ID="$(. /etc/os-release && printf '%s' "${ID:-}")"
  CODENAME="$(. /etc/os-release && printf '%s' "${VERSION_CODENAME:-}")"
fi
if [ "$OS_ID" != "ubuntu" ] || [ -z "$CODENAME" ]; then
  echo "setup_git_apt_repo: not Ubuntu (ID=${OS_ID:-?}, codename=${CODENAME:-?}); the git-core PPA has no builds here, skipping." >&2
  exit 0
fi
if ! command -v gpg >/dev/null 2>&1; then
  echo "setup_git_apt_repo: gpg is required to dearmor the signing key; skipping." >&2
  exit 1
fi
if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -nv -O "$2" "$1"; }
else
  echo "setup_git_apt_repo: need curl or wget to fetch the signing key; skipping." >&2
  exit 1
fi

write_prefs() {
  if [ -f "$PREFS" ] && [ "${GIT_REPO_FORCE:-0}" != "1" ] &&
     grep -q 'ppa\.launchpadcontent\.net' "$PREFS" 2>/dev/null; then
    return 0
  fi
  echo "setup_git_apt_repo: pinning git to the git-core PPA (above Ubuntu Pro ESM)..."
  sudo mkdir -p -m 755 /etc/apt/preferences.d || return 1
  cat <<'PREF_EOF' | sudo tee "$PREFS" >/dev/null || return 1
Package: git git-man
Pin: origin ppa.launchpadcontent.net
Pin-Priority: 600
PREF_EOF
  return 0
}

# refresh_key re-fetches on every run: a rotated upstream key otherwise leaves
# every apt update failing signature checks until someone notices.
refresh_key() {
  local tmpkey
  tmpkey="$(mktemp)" || return 1
  trap 'rm -f "$tmpkey"' RETURN
  if ! fetch "$KEY_URL" "$tmpkey"; then
    echo "setup_git_apt_repo: failed to download the signing key from the keyserver." >&2
    return 1
  fi
  sudo mkdir -p -m 755 /etc/apt/keyrings || return 1
  gpg --dearmor < "$tmpkey" | sudo tee "$KEYRING" >/dev/null || return 1
  sudo chmod 0644 "$KEYRING" || return 1
}

if [ -f "$KEYRING" ] && [ -f "$SOURCES" ] && [ "${GIT_REPO_FORCE:-0}" != "1" ]; then
  refresh_key || {
    echo "setup_git_apt_repo: could not refresh $KEYRING; apt signature checks against the PPA may keep failing after a key rotation." >&2
    exit 1
  }
  write_prefs || {
    echo "setup_git_apt_repo: could not write $PREFS; git would stay pinned to ESM/universe." >&2
    exit 1
  }
  echo "setup_git_apt_repo: git-core PPA already configured; refreshed the signing key (GIT_REPO_FORCE=1 to also rewrite the repo entry)."
  exit 0
fi

echo "setup_git_apt_repo: configuring the git-core PPA for a current git..."
refresh_key || exit 1
echo "deb [arch=$(dpkg --print-architecture) signed-by=${KEYRING}] ${REPO_URL} ${CODENAME} main" \
  | sudo tee "$SOURCES" >/dev/null || exit 1
write_prefs || exit 1
echo "setup_git_apt_repo: done — the next apt-get update + install git resolves to the PPA build."
