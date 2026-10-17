#!/usr/bin/env bash
# install_htop.sh — the dotfiles htop setup (gff install.tools.htop-fit).
#
#   install_htop.sh [install]   # flag on  (install.sh --phase config)
#   install_htop.sh remove      # flag off: return htop to its built-in defaults
#
# install:
#   * copies opt/etc/htop/htop-fit.sh to ~/.config/htop/htop-fit.sh, which
#     ~/.zshrc and ~/.bashrc source: an htop() wrapper that fills the header
#     with history graphs so only the top few processes show (see that file);
#   * installs opt/etc/htop/htoprc as ~/.config/htop/htoprc (htop's default
#     two-column meters, CPU% sort, no tree view). An existing htoprc that
#     dotfiles does not manage is first kept as htoprc.pre-dotfiles.bak.
#     A marker (.dotfiles-managed) records the cksum of what was installed; a
#     later run updates the file only while it still matches, so changes made
#     with `command htop` survive (HTOP_CONFIG_FORCE=1 replaces them anyway,
#     keeping a .bak).
#
# remove:
#   * deletes the wrapper, and moves a dotfiles-managed htoprc aside to
#     htoprc.dotfiles.bak so htop starts from its built-in defaults. An htoprc
#     dotfiles never managed is left alone.
#
# Env: HTOP_CONFIG_DIR (default ${XDG_CONFIG_HOME:-~/.config}/htop),
#      HTOP_CONFIG_FORCE=1.
set -eu

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
SRC_DIR="${REPO_ROOT}/opt/etc/htop"
DEST_DIR="${HTOP_CONFIG_DIR:-${XDG_CONFIG_HOME:-${HOME}/.config}/htop}"
RC="${DEST_DIR}/htoprc"
FIT="${DEST_DIR}/htop-fit.sh"
MARKER="${DEST_DIR}/.dotfiles-managed"

info() { echo "htop: $*"; }

sum_of() { cksum <"$1" | awk '{print $1 "-" $2}'; }

# True when RC is the file dotfiles last installed, unchanged since.
rc_is_managed_and_pristine() {
  [ -f "${MARKER}" ] && [ -f "${RC}" ] && [ "$(cat "${MARKER}")" = "$(sum_of "${RC}")" ]
}

do_install() {
  mkdir -p "${DEST_DIR}"
  cp "${SRC_DIR}/htop-fit.sh" "${FIT}"
  info "installed ${FIT}"

  if [ -f "${RC}" ] && cmp -s "${SRC_DIR}/htoprc" "${RC}"; then
    sum_of "${RC}" >"${MARKER}"
    info "${RC} is current"
    return 0
  fi
  if [ ! -f "${RC}" ]; then
    :
  elif rc_is_managed_and_pristine; then
    info "updating managed ${RC}"
  elif [ -f "${MARKER}" ] && [ "${HTOP_CONFIG_FORCE:-0}" != "1" ]; then
    info "${RC} was changed since dotfiles installed it; leaving it alone (HTOP_CONFIG_FORCE=1 replaces it, keeping a .bak)."
    return 0
  elif [ -f "${MARKER}" ]; then
    cp "${RC}" "${RC}.bak"
    info "replacing changed ${RC} (kept ${RC}.bak)"
  else
    cp "${RC}" "${RC}.pre-dotfiles.bak"
    info "kept your previous htoprc as ${RC}.pre-dotfiles.bak"
  fi
  cp "${SRC_DIR}/htoprc" "${RC}"
  sum_of "${RC}" >"${MARKER}"
  info "installed ${RC}"
}

do_remove() {
  if [ -f "${FIT}" ]; then
    rm -f "${FIT}"
    info "removed ${FIT}"
  fi
  if [ -f "${MARKER}" ]; then
    if [ -f "${RC}" ]; then
      mv "${RC}" "${RC}.dotfiles.bak"
      info "moved the dotfiles htoprc to ${RC}.dotfiles.bak; htop now starts from its defaults"
    fi
    rm -f "${MARKER}"
  fi
}

case "${1:-install}" in
  install) do_install ;;
  remove) do_remove ;;
  *)
    echo "usage: $0 [install|remove]" >&2
    exit 2
    ;;
esac
