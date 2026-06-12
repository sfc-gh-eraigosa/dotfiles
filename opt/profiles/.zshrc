#!/bin/zsh
# Path to your oh-my-zsh installation.

# Detect if we're in VSCode/Cursor terminal
if [[ "$TERM_PROGRAM" == "vscode" ]] || [[ "$TERM_PROGRAM" == "cursor" ]] || [[ -n "$VSCODE_PID" ]] || [[ -n "$CURSOR_PID" ]]; then
    export EDITOR_TERMINAL=true
else
    export EDITOR_TERMINAL=false
fi

# Daily maintenance cache (run expensive tasks at most once per 24 hours)
# Uses a simple mtime check on a stamp file under XDG cache or ~/.cache
DAILY_CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/dotfiles"
DAILY_STAMP_FILE="${DAILY_CACHE_DIR}/daily_maintenance.stamp"

should_run_daily_maintenance() {
  [ -d "${DAILY_CACHE_DIR}" ] || mkdir -p "${DAILY_CACHE_DIR}"
  # If the stamp file doesn't exist, run maintenance
  [ -f "${DAILY_STAMP_FILE}" ] || return 0
  # macOS: stat -f %m returns epoch seconds of mtime
  local now
  local mtime
  now=$(date +%s)
  if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS
    mtime=$(stat -f %m "${DAILY_STAMP_FILE}" 2>/dev/null || echo 0)
  else
    # Linux / Raspberry Pi
    mtime=$(stat -c %Y "${DAILY_STAMP_FILE}" 2>/dev/null || echo 0)
  fi
  [ $(( now - mtime )) -ge 86400 ]
}

touch_daily_maintenance_stamp() {
  # Update/touch stamp before launching maintenance to prevent duplicate runs
  : > "${DAILY_STAMP_FILE}" 2>/dev/null || touch "${DAILY_STAMP_FILE}" 2>/dev/null
}

# Expensive operations moved under daily maintenance above for faster startup
run_daily_maintenance() {
  # Pull the latest repos (previously in startup)
  if [ -f "${HOME}/.gitrepos" ] ; then
    (
      cd "${HOME}" || exit 0
      [ -d "${HOME}/.git" ] && \
        GIT_TERMINAL_PROMPT=0 git pull origin "$(git branch | grep '*' | awk '{print $2}')" 2>/dev/null
      GIT_TERMINAL_PROMPT=0 "${HOME}/.gitrepos" > /dev/null 2>&1
    )
  fi
  # Setup any missing brew packages from the $HOME/Brewfile
  if [ "$(uname -s)" = "Darwin" ] && command -v brew >/dev/null 2>&1 ; then
    brew bundle check || brew bundle
  fi
}

# Trigger daily maintenance only in non-editor terminals
if [[ "$EDITOR_TERMINAL" == "false" ]]; then
  if should_run_daily_maintenance; then
    touch_daily_maintenance_stamp
    run_daily_maintenance &
  fi
fi

if [ -f $HOME/.goenv.sh ]; then
    . $HOME/.goenv.sh
fi

if [ -f ~/.bash_aliases ]; then
    . ~/.bash_aliases
fi

# echo 'POWERLEVEL9K_DISABLE_CONFIGURATION_WIZARD=true'
# To customize prompt, run `p10k configure` or edit ~/.p10k.zsh.
[[ ! -f ~/.p10k.zsh ]] || source ~/.p10k.zsh

DEFAULT_USER=docker
export Z_HOME=$HOME

if [ -d "${Z_HOME}/.oh-my-zsh" ] ; then
  export ZSH="${Z_HOME}/.oh-my-zsh"
else
  export ZSH="${Z_HOME}/git/oh-my-zsh"
fi

if [ -f ~/opt/themes/agnoster.zsh-theme ] ; then
  cp ~/opt/themes/agnoster.zsh-theme "$ZSH/themes/agnoster.zsh-theme"
fi

# Set name of the theme to load.
# Look in ~/.oh-my-zsh/themes/
# Optionally, if you set this to "random", it'll load a random theme each
# time that oh-my-zsh is loaded.
ZSH_THEME="agnoster"

# Uncomment the following line to use case-sensitive completion.
# CASE_SENSITIVE="true"

# Uncomment the following line to use hyphen-insensitive completion. Case
# sensitive completion must be off. _ and - will be interchangeable.
# HYPHEN_INSENSITIVE="true"

# Uncomment the following line to disable bi-weekly auto-update checks.
# DISABLE_AUTO_UPDATE="true"

# Uncomment the following line to change how often to auto-update (in days).
# export UPDATE_ZSH_DAYS=13

# Uncomment the following line to disable colors in ls.
# DISABLE_LS_COLORS="true"

# Uncomment the following line to disable auto-setting terminal title.
# DISABLE_AUTO_TITLE="true"

# Uncomment the following line to enable command auto-correction.
# ENABLE_CORRECTION="true"

# Uncomment the following line to display red dots whilst waiting for completion.
# COMPLETION_WAITING_DOTS="true"

# Uncomment the following line if you want to disable marking untracked files
# under VCS as dirty. This makes repository status check for large repositories
# much, much faster.
# DISABLE_UNTRACKED_FILES_DIRTY="true"

# Uncomment the following line if you want to change the command execution time
# stamp shown in the history command output.
# The optional three formats: "mm/dd/yyyy"|"dd.mm.yyyy"|"yyyy-mm-dd"
# HIST_STAMPS="mm/dd/yyyy"

# Would you like to use another custom folder than $ZSH/custom?
# ZSH_CUSTOM=/path/to/new-custom-folder

# Which plugins would you like to load? (plugins can be found in ~/.oh-my-zsh/plugins/*)
# Custom plugins may be added to ~/.oh-my-zsh/custom/plugins/
# Example format: plugins=(rails git textmate ruby lighthouse)
# Add wisely, as too many plugins slow down shell startup.
# plugins=(git)
plugins=(git docker golang zsh-completions kubectl)

# Only clone zsh-completions if not in editor terminal (expensive git operation)
if [[ "$EDITOR_TERMINAL" == "false" ]] && [ ! -d "${ZSH_CUSTOM:-${ZSH:-~/.oh-my-zsh}/custom}/plugins/zsh-completions" ] ; then
  git clone https://github.com/zsh-users/zsh-completions ${ZSH_CUSTOM:-${ZSH:-~/.oh-my-zsh}/custom}/plugins/zsh-completions
fi

# User configuration

# export PATH="$PATH:$HOME/opt/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:$HOME/go/bin"
export PATH="$PATH:$HOME/bin"
if [ -d "${HOME}/opt/bin" ] ; then
    PATH="${HOME}/opt/bin:$PATH"
fi
if [ -d "${HOME}/opt/google-cloud-sdk/bin" ] ; then
    PATH="${HOME}/opt/google-cloud-sdk/bin:$PATH"
fi
if [ -d "${HOME}/opt/scripts" ] ; then
    for d in "${HOME}/opt/scripts"/*; do
        if [ -d "$d" ]; then
            PATH="$d:$PATH"
        fi
    done
fi
# export MANPATH="/usr/local/man:$MANPATH"
fpath+=${ZSH_CUSTOM:-${ZSH:-~/.oh-my-zsh}/custom}/plugins/zsh-completions/src

# Shadow dangling completion symlinks before oh-my-zsh runs compinit. Vendor
# packages such as Docker Desktop on WSL leave _* symlinks in root-owned $fpath
# dirs that dangle when their backing mount is gone; compinit then errors
# ("no such file or directory") reading the broken link. An empty stub earlier
# in $fpath makes compinit skip it. Rebuilt each startup, so the real
# completion is used again once its target returns.
# NOTE: this only protects OUR compinit (oh-my-zsh + the cached one below). The
# distro's GLOBAL compinit in /etc/zsh/zshrc runs before this file, so that one
# is disabled separately via `skip_global_compinit=1` in ~/.zshenv.
_comp_shadow="${XDG_CACHE_HOME:-$HOME/.cache}/zsh/compshadow"
rm -rf "$_comp_shadow" 2>/dev/null
for _comp_f in ${^fpath}/_*(N@); do
  [[ -e $_comp_f ]] && continue                 # symlink target resolves: leave it
  [[ -d $_comp_shadow ]] || mkdir -p "$_comp_shadow"
  : > "$_comp_shadow/${_comp_f:t}"
done
[[ -d $_comp_shadow ]] && fpath=("$_comp_shadow" $fpath)
unset _comp_f _comp_shadow

source $ZSH/oh-my-zsh.sh

# oh-my-zsh's git plugin defines `alias gss='git status -s'` which shadows
# our gss binary at ~/opt/bin/gss. Drop the alias so the binary wins —
# critical for any AI assistant (Claude, Gemini) that calls `gss push`.
unalias gss 2>/dev/null

# Claude Code CLI helpers: claude (wrapper) and claude-toggle (YOLO on/off)
[ -f "${HOME}/.config/claude/aliases.sh" ] && . "${HOME}/.config/claude/aliases.sh"

# You may need to manually set your language environment
# export LANG=en_US.UTF-8

# Preferred editor for local and remote sessions
# if [[ -n $SSH_CONNECTION ]]; then
#   export EDITOR='vim'
# else
#   export EDITOR='mvim'
# fi

# Compilation flags
# export ARCHFLAGS="-arch x86_64"

# ssh
# export SSH_KEY_PATH="~/.ssh/dsa_id"

# Set personal aliases, overriding those provided by oh-my-zsh libs,
# plugins, and themes. Aliases can be placed here, though oh-my-zsh
# users are encouraged to define aliases within the ZSH_CUSTOM folder.
# For a full list of active aliases, run `alias`.
#
# Example aliases
# alias zshconfig="mate ~/.zshrc"
# alias ohmyzsh="mate ~/.oh-my-zsh"
if [ -f ~/.zsh/zsh-autosuggestions/zsh-autosuggestions.zsh ] ; then
  .  ~/.zsh/zsh-autosuggestions/zsh-autosuggestions.zsh
  export ZSH_AUTOSUGGEST_HIGHLIGHT_STYLE=fg=5
fi

if [ -f ~/.zsh/dircolors-solarized/dircolors.ansi-universal ] ; then
  eval "$(dircolors ~/.zsh/dircolors-solarized/dircolors.ansi-dark)"
fi

export PATH="$PATH:$HOME/.rvm/bin" # Add RVM to PATH for scripting

# Initialize nodenv with lazy loading for better performance
if command -v nodenv &> /dev/null; then
  if [[ "$EDITOR_TERMINAL" == "true" ]]; then
    # Fast loading in editor terminals - just add to PATH
    export PATH="$HOME/.nodenv/shims:$PATH"
  else
    # Full initialization in regular terminals
    eval "$(nodenv init -)"
  fi
fi

if [ -f "$HOME/git/powerlevel10k/powerlevel10k.zsh-theme" ]; then
  source "$HOME/git/powerlevel10k/powerlevel10k.zsh-theme"
fi

#THIS MUST BE AT THE END OF THE FILE FOR SDKMAN TO WORK!!!
# Lazy load SDKMAN for better performance in editor terminals
if [ -d "${HOME}/.sdkman" ] ; then
  export SDKMAN_DIR="${HOME}/.sdkman"
  if [[ "$EDITOR_TERMINAL" == "true" ]]; then
    # Fast loading - just set up environment
    export PATH="$SDKMAN_DIR/bin:$PATH"
  else
    # Full initialization in regular terminals
    [[ -s "${HOME}/.sdkman/bin/sdkman-init.sh" ]] && source "${HOME}/.sdkman/bin/sdkman-init.sh"
  fi
fi
set -o vi
unset GIT_URL
test -f ${HOME}/.rbenv/shims/gh && rm -f ${HOME}/.rbenv/shims/gh

#
# node in path
export PATH="$PATH:$HOME/.nodenv/shims"

#
# GITHUB_TOKEN setup from stored credential (skip expensive git credential call in editor terminals)
if [[ ! -f .no_github_token ]] ; then
  if [[ "$EDITOR_TERMINAL" == "true" ]] && [[ -n "$GITHUB_TOKEN" ]]; then
    # Use existing GITHUB_TOKEN if available
    export GITHUB_TOKEN
  elif [[ "$EDITOR_TERMINAL" == "false" ]]; then
    # Only fetch credentials in regular terminals
    export GITHUB_TOKEN=${GITHUB_TOKEN:-$( printf "protocol=https\\nhost=github.com\\npath=github\\n" | GIT_TERMINAL_PROMPT=0 git credential fill 2>/dev/null | awk -F'=' '/password=/{print $2}')}
  fi
fi

test -n "$(alias ruby)" && unalias ruby

#
# we're in codespaces, setup some custom things
# change to workspace if defined
if [ -d "${CODESPACE_VSCODE_FOLDER}" ]; then
  cd "${CODESPACE_VSCODE_FOLDER}"
  git config pull.rebase true
  if [ -n "${MY_GITHUB_TOKEN}" ] ; then
    export GITHUB_TOKEN="${MY_GITHUB_TOKEN}"
  fi
fi

# source any secrets
if [ -f ~/.secrets.env ] ; then
  echo "sourcing secrets"
  source ~/.secrets.env
fi

# autoload -Uz compinit
# compinit

# Created by `pipx` on 2024-04-03 04:50:34
export PATH=${PATH}:${HOME}/.local/bin
# Load direnv conditionally for performance
if command -v direnv &> /dev/null; then
  if [[ "$EDITOR_TERMINAL" == "true" ]]; then
    # Simplified direnv setup for editor terminals
    export _DIRENV_HOOK=1
  else
    # Full direnv initialization
    eval "$(direnv hook zsh)"
  fi
fi

# >>> conda initialize >>>
# !! Contents within this block are managed by 'conda init' !!
# Conditional conda loading for better performance
if [[ "$EDITOR_TERMINAL" == "true" ]]; then
    # Fast loading - just add conda to PATH
    export PATH="/opt/homebrew/Caskroom/miniconda/base/bin:$PATH"
else
    # Full conda initialization for regular terminals
    __conda_setup="$('/opt/homebrew/Caskroom/miniconda/base/bin/conda' 'shell.zsh' 'hook' 2> /dev/null)"
    if [ $? -eq 0 ]; then
        eval "$__conda_setup"
    else
        if [ -f "/opt/homebrew/Caskroom/miniconda/base/etc/profile.d/conda.sh" ]; then
            . "/opt/homebrew/Caskroom/miniconda/base/etc/profile.d/conda.sh"
        else
            export PATH="/opt/homebrew/Caskroom/miniconda/base/bin:$PATH"
        fi
    fi
    unset __conda_setup
fi
# <<< conda initialize <<<

export PATH="${KREW_ROOT:-$HOME/.krew}/bin:$PATH"
# Load SF aliases conditionally
if command -v sf &> /dev/null; then
  if [[ "$EDITOR_TERMINAL" == "false" ]]; then
    eval "$(sf aliases)"
  fi
fi
export PATH="$PATH:$HOME/go/bin"
# Fix Docker permissions if needed (Linux only)
if [[ "$(uname -s)" == "Linux" ]] && [[ "$EDITOR_TERMINAL" == "false" ]]; then
  if [ -S /var/run/docker.sock ] && [ ! -w /var/run/docker.sock ]; then
    echo "Fixing Docker socket permissions (sudo chmod 666 /var/run/docker.sock)..."
    sudo chmod 666 /var/run/docker.sock 2>/dev/null
  fi
fi

# The following lines have been added by Docker Desktop to enable Docker CLI completions.
[ -d "$HOME/.docker/completions" ] && fpath=($HOME/.docker/completions $fpath)
fpath+=~/.zsh/completions
# autoload -Uz compinit
# compinit
# End of Docker CLI completions
# pyenv setup
export PYENV_ROOT="$HOME/.pyenv"
[[ -d $PYENV_ROOT/bin ]] && export PATH="$PYENV_ROOT/bin:$PATH"
if command -v pyenv &>/dev/null; then
  eval "$(pyenv init - zsh)"
fi

# rbenv setup
[[ -d $HOME/.rbenv/bin ]] && export PATH="$HOME/.rbenv/bin:$PATH"
if command -v rbenv &>/dev/null; then
  eval "$(rbenv init - zsh)"
fi

# fnm setup
[[ -d "$HOME/.local/share/fnm" ]] && export PATH="$HOME/.local/share/fnm:$PATH"
if command -v fnm &>/dev/null; then
  eval "$(fnm env --use-on-cd)"
fi

# Cortex CLI completion (disable via /settings in cortex)
[[ -s ~/.zsh/completions/cortex.zsh ]] && source ~/.zsh/completions/cortex.zsh

# added by Snowflake SnowSQL installer v1.2
if [ -d "/Applications/SnowSQL.app/Contents/MacOS" ]; then
  export PATH="/Applications/SnowSQL.app/Contents/MacOS:$PATH"
fi

# OpenClaw Integration
# [ -f "$HOME/.openclaw.sh" ] && source "$HOME/.openclaw.sh"

# OpenClaw Completion

# --- Performance Optimizations by Gemini ---

# Optimize compinit to run once per day
autoload -Uz compinit
_comp_dumpfile="${ZSH_COMPDUMP:-$HOME/.zcompdump}"
if [[ "$OSTYPE" == "darwin"* ]]; then
  _comp_mtime=$(stat -f %m "$_comp_dumpfile" 2>/dev/null || echo 0)
else
  _comp_mtime=$(stat -c %Y "$_comp_dumpfile" 2>/dev/null || echo 0)
fi

if (( $(date +%s) - _comp_mtime > 86400 )); then
  compinit
else
  compinit -C
fi
unset _comp_dumpfile _comp_mtime

# goenv Lazy Loader
go() {
  unset -f go goenv
  [ -f "$HOME/.goenv.sh" ] && source "$HOME/.goenv.sh"
  go "$@"
}
goenv() {
  unset -f go goenv
  [ -f "$HOME/.goenv.sh" ] && source "$HOME/.goenv.sh"
  goenv "$@"
}

# OpenClaw Lazy Completion
openclaw() {
  unset -f openclaw
  if [ -f "$HOME/.openclaw/completions/openclaw.zsh" ]; then
    source "$HOME/.openclaw/completions/openclaw.zsh"
  elif command -v openclaw >/dev/null; then
    source <(openclaw completion --shell zsh)
  fi
  openclaw "$@"
}

# Added by tmux-mgr
[ -f ${HOME}/.config/tmux-mgr/aliases.sh ] && source ${HOME}/.config/tmux-mgr/aliases.sh

# Load Gemini CLI environment
[[ -f "$HOME/.gemini.profile" ]] && source "$HOME/.gemini.profile"
# Gemini CLI helpers: gemini() wrapper with tmux auto-anchor
[ -f "${HOME}/.config/gemini/aliases.sh" ] && . "${HOME}/.config/gemini/aliases.sh"

# Load Nano Platform environment
[ -f "$HOME/.nano_profile" ] && . "$HOME/.nano_profile"

# Machine-local overrides (not tracked in dotfiles — safe to create on any host)
# Use this file for host-specific aliases, PATH entries, or function overrides
# that should not be committed to the shared repo.
# See: docs/machine-local-overrides.md
[ -f "$HOME/.zshrc.local" ] && source "$HOME/.zshrc.local"
true
