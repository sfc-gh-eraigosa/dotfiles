# shellcheck shell=sh
# .locale.sh — sourced helper: fall back to a locale this host actually has.
#
# ssh forwards the client's LANG/LC_* (Ubuntu's and macOS's default SendEnv
# LANG LC_*, accepted by Ubuntu's sshd), so a session can arrive with a locale
# the host never generated: LC_ALL=en_US.UTF-8 from a Linux box, or
# LC_CTYPE=UTF-8 from macOS Terminal. Every bash, perl and python started from
# there then warns "setlocale: LC_ALL: cannot change locale" — 95 times in one
# fleet run of install.sh, and once even inside goenv's GOROOT path.
#
# locale_fallback peels only the broken layer, stopping as soon as `locale` is
# happy: LC_ALL first, then whichever LC_* are set, and LANG (to C.UTF-8, else
# C) only when LANG itself is the problem. A working locale costs one `locale`
# call and changes nothing.
#
# POSIX sh on purpose: .profile is read by dash at a Pi's GUI login, where a
# bash-ism is a parse error that aborts the session (the login-loop incident),
# and .zshrc / .bashrc / install.sh source the same file.
#
# Usage:  . ~/.locale.sh && locale_fallback [-q]     (-q: no message)
# The shell that sourced this already printed its own startup warning; only
# what it runs afterwards is fixed.

locale_fallback() {
  command -v locale >/dev/null 2>&1 || return 0
  [ -n "$(locale 2>&1 >/dev/null)" ] || return 0
  _lf_drop="${LC_ALL:+LC_ALL=${LC_ALL}}"
  unset LC_ALL
  if [ -n "$(locale 2>&1 >/dev/null)" ]; then
    for _lf_v in LC_CTYPE LC_NUMERIC LC_TIME LC_COLLATE LC_MONETARY LC_MESSAGES \
      LC_PAPER LC_NAME LC_ADDRESS LC_TELEPHONE LC_MEASUREMENT LC_IDENTIFICATION; do
      # Only what is set: unsetting an absent LC_* makes bash re-run setlocale
      # and warn once per category while LANG is still the broken one.
      eval "_lf_val=\${${_lf_v}:-}"
      if [ -n "${_lf_val}" ]; then
        _lf_drop="${_lf_drop:+${_lf_drop} }${_lf_v}=${_lf_val}"
        unset "${_lf_v}"
      fi
    done
    if [ -n "$(locale 2>&1 >/dev/null)" ]; then
      _lf_drop="${_lf_drop:+${_lf_drop} }LANG=${LANG:-}"
      if locale -a 2>/dev/null | grep -qix 'c\.utf-\{0,1\}8'; then LANG=C.UTF-8; else LANG=C; fi
      export LANG
      unset LANGUAGE
    fi
  fi
  if [ "${1:-}" != "-q" ]; then
    echo "locale: not installed on this host; dropped ${_lf_drop}, using LANG=${LANG:-C}."
  fi
  unset _lf_drop _lf_v _lf_val
  return 0
}
