#!/usr/bin/env bash
# setup_jetson_browser.sh — install Chromium on a Jetson and make it the default
# browser. Called from install.sh's Jetson block (the caller checks is_jetson).
#
# The package is `chromium`, a real .deb from a PPA (xtradeb / savoury1), never
# Ubuntu's `chromium-browser`: on 22.04 (L4T) that is a snap transitional
# package, and snaps do not run on Jetson. With no `chromium` candidate there is
# nothing safe to install, so this says why and skips.
#
# Idempotent and quiet on an up-to-date host: an installed package and an
# already-set default call neither apt nor sudo. Every failure is a WARNING and
# exit 0 — a browser must never abort the installer.
#
# CHROMIUM_BIN overrides the binary path (tests point it at a fixture).
set -u

CHROMIUM_BIN="${CHROMIUM_BIN:-/usr/bin/chromium}"

if ! command -v apt-get >/dev/null 2>&1 || ! command -v apt-cache >/dev/null 2>&1; then
  echo "No apt on this host; skipping the Chromium setup."
  exit 0
fi

echo "Ensuring Chromium is installed and set as default..."

if [ "$(dpkg-query -W -f='${Status}' chromium 2>/dev/null)" = "install ok installed" ]; then
  echo "Chromium already installed."
else
  candidate="$(apt-cache policy chromium 2>/dev/null | awk '/Candidate:/ { print $2; exit }')"
  if [ -z "${candidate}" ] || [ "${candidate}" = "(none)" ]; then
    echo "No installable chromium .deb (Ubuntu's chromium-browser is a snap transitional"
    echo "package, which does not run on Jetson). Add a chromium PPA (e.g. xtradeb/apps)"
    echo "to install it. Skipping."
    exit 0
  fi
  # DEBIAN_FRONTEND=noninteractive on the sudo env is load-bearing: a debconf
  # prompt (tzdata-class) blocks forever without a tty (the Docker Image CI hang,
  # PR #182).
  if ! sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq chromium; then
    echo "WARNING: could not install chromium; leaving the default browser unchanged."
    exit 0
  fi
fi

if [ ! -x "${CHROMIUM_BIN}" ]; then
  echo "WARNING: ${CHROMIUM_BIN} not found after install; leaving the default browser unchanged."
  exit 0
fi

# update-alternatives needs root; skip it when the choice is already ours.
for _alt in x-www-browser gnome-www-browser; do
  _current="$(update-alternatives --query "${_alt}" 2>/dev/null | awk '/^Value:/ { print $2; exit }')"
  if [ "${_current}" != "${CHROMIUM_BIN}" ]; then
    sudo update-alternatives --set "${_alt}" "${CHROMIUM_BIN}" 2>/dev/null || true
  fi
done

# The per-user desktop default (no root needed).
if command -v xdg-settings >/dev/null 2>&1 &&
  [ "$(xdg-settings get default-web-browser 2>/dev/null)" != "chromium.desktop" ]; then
  xdg-settings set default-web-browser chromium.desktop 2>/dev/null || true
fi
