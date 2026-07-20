# shellcheck shell=sh
# ~/.zprofile — zsh LOGIN shells read this; they never read ~/.profile
# (that's a bash/sh file) and non-interactive login shells (`zsh -lc`,
# SSH remote commands, GUI/automation launchers) never read ~/.zshrc
# either. Without this file, any zsh login shell that isn't interactive
# gets NO dotfiles environment at all — no ~/opt/bin, no scripts dirs,
# no version managers — which is exactly how automation-launched
# sessions ended up with a bare PATH.
#
# Delegate to ~/.profile (the canonical, POSIX-only env setup shared
# with bash/dash) under sh emulation, so its POSIX semantics (word
# splitting etc.) hold even though zsh is interpreting it. ~/.zshrc
# still layers interactive-only setup (completion, prompt, aliases) on
# top for interactive shells.
[ -f "$HOME/.profile" ] && emulate sh -c '. "$HOME/.profile"'
