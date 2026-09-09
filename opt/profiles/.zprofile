# shellcheck shell=sh
# ~/.zprofile — zsh LOGIN shells read this; they never read ~/.profile
# (that's a bash/sh file) and non-interactive login shells (`zsh -lc`,
# SSH remote commands, GUI/automation launchers) never read ~/.zshrc
# either. Without this file, any zsh login shell that isn't interactive
# gets NO dotfiles environment at all — no ~/opt/bin, no scripts dirs,
# no version managers — which is exactly how automation-launched
# sessions ended up with a bare PATH.
#
# Ubuntu's stock /etc/zsh/zprofile is comment-only on some releases (verified
# on DGX OS 7.x / Ubuntu 24.04), so a zsh login shell never sources
# /etc/profile — and therefore never runs ANY /etc/profile.d/*.sh drop-in.
# Vendors ship real PATH setup there: NVIDIA's nv_paths.sh is what puts
# /usr/local/cuda/bin (nvcc, ncu, cuda-gdb) on PATH. bash login shells got
# it and zsh did not, which is a silent, shell-dependent environment split.
#
# Source it ourselves under sh emulation so zsh follows the same order bash
# does: /etc/profile (+ profile.d) first, then ~/.profile. /etc/profile only
# pulls in /etc/bash.bashrc when $BASH is set, so this stays inert under zsh.
# Re-sourcing when the system zprofile DID already do it is harmless: the
# PATH dedupe at the end of ~/.profile collapses any repeated entries.
[ -r /etc/profile ] && emulate sh -c '. /etc/profile'

# Delegate to ~/.profile (the canonical, POSIX-only env setup shared
# with bash/dash) under sh emulation, so its POSIX semantics (word
# splitting etc.) hold even though zsh is interpreting it. ~/.zshrc
# still layers interactive-only setup (completion, prompt, aliases) on
# top for interactive shells.
[ -f "$HOME/.profile" ] && emulate sh -c '. "$HOME/.profile"'
