# ~/.bashrc: executed by bash(1) for non-login shells.

# Detect if we're in VSCode/Cursor terminal
if [[ "$TERM_PROGRAM" == "vscode" ]] || [[ "$TERM_PROGRAM" == "cursor" ]] || [[ -n "$VSCODE_PID" ]] || [[ -n "$CURSOR_PID" ]]; then
    export EDITOR_TERMINAL=true
else
    export EDITOR_TERMINAL=false
fi

# Daily maintenance (bash) - align with zsh once-per-day behavior
DOTFILES_DAILY_CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/dotfiles"
DOTFILES_DAILY_STAMP_FILE="${DOTFILES_DAILY_CACHE_DIR}/daily_maintenance.stamp"
dotfiles_should_run_daily() {
    [ -d "${DOTFILES_DAILY_CACHE_DIR}" ] || mkdir -p "${DOTFILES_DAILY_CACHE_DIR}"
    [ -f "${DOTFILES_DAILY_STAMP_FILE}" ] || return 0
    local now mtime
    now=$(date +%s)
    if mtime=$(stat -f %m "${DOTFILES_DAILY_STAMP_FILE}" 2>/dev/null); then
        :
    else
        mtime=$(stat -c %Y "${DOTFILES_DAILY_STAMP_FILE}" 2>/dev/null || echo 0)
    fi
    [ $(( now - mtime )) -ge 86400 ]
}
dotfiles_touch_daily() {
    : > "${DOTFILES_DAILY_STAMP_FILE}" 2>/dev/null || touch "${DOTFILES_DAILY_STAMP_FILE}" 2>/dev/null
}
dotfiles_run_daily_maintenance() {
    # Pull the latest repos
    if [ -f "${HOME}/.gitrepos" ] ; then
      (
        cd "${HOME}" || exit 0
        [ -d "${HOME}/.git" ] && \
          GIT_TERMINAL_PROMPT=0 git pull origin "$(git branch | grep '*' | awk '{print $2}')" 2>/dev/null
        GIT_TERMINAL_PROMPT=0 "${HOME}/.gitrepos" > /dev/null 2>&1
      )
    fi
    # Load SF aliases (can be slow)
    if command -v sf >/dev/null 2>&1 ; then
      eval "$(sf aliases)"
    fi
}
if [[ "$EDITOR_TERMINAL" == "false" ]]; then
    if dotfiles_should_run_daily; then
        dotfiles_touch_daily
        dotfiles_run_daily_maintenance &
    fi
fi

# Skip debug output in editor terminals
if [[ "$EDITOR_TERMINAL" == "false" ]]; then
    echo executing bashrc
fi

# Expensive repo update moved to daily maintenance above

if [ -f $HOME/.goenv.sh ]; then
    . $HOME/.goenv.sh
fi

if [ -f ~/.bash_aliases ]; then
    . ~/.bash_aliases
fi

# Fix Docker permissions if needed (Linux only)
if [[ "$(uname -s)" == "Linux" ]] && [[ "$EDITOR_TERMINAL" == "false" ]]; then
  if [ -S /var/run/docker.sock ] && [ ! -w /var/run/docker.sock ]; then
    echo "Fixing Docker socket permissions (sudo chmod 666 /var/run/docker.sock)..."
    sudo chmod 666 /var/run/docker.sock 2>/dev/null
  fi
fi

# see /usr/share/doc/bash/examples/startup-files (in the package bash-doc)
# for examples

# If not running interactively, don't do anything
case $- in
    *i*) ;;
      *) return;;
esac

# don't put duplicate lines or lines starting with space in the history.
# See bash(1) for more options
HISTCONTROL=ignoreboth

# append to the history file, don't overwrite it
if [ ! -z "$(shopt -p|grep shopt)" ] ; then
    shopt -s histappend
fi

# for setting history length see HISTSIZE and HISTFILESIZE in bash(1)
HISTSIZE=1000
HISTFILESIZE=2000

# check the window size after each command and, if necessary,
# update the values of LINES and COLUMNS.
if [ ! -z "$(shopt -p|grep shopt)" ] ; then
    shopt -s checkwinsize
fi

# If set, the pattern "**" used in a pathname expansion context will
# match all files and zero or more directories and subdirectories.
#shopt -s globstar

# make less more friendly for non-text input files, see lesspipe(1)
[ -x /usr/bin/lesspipe ] && eval "$(SHELL=/bin/sh lesspipe)"

# set variable identifying the chroot you work in (used in the prompt below)
if [ -z "${debian_chroot:-}" ] && [ -r /etc/debian_chroot ]; then
    debian_chroot=$(cat /etc/debian_chroot)
fi

# set a fancy prompt (non-color, unless we know we "want" color)
case "$TERM" in
    xterm-color) color_prompt=yes;;
esac

# uncomment for a colored prompt, if the terminal has the capability; turned
# off by default to not distract the user: the focus in a terminal window
# should be on the output of commands, not on the prompt
#force_color_prompt=yes

if [ -n "$force_color_prompt" ]; then
    if [ -x /usr/bin/tput ] && tput setaf 1 >&/dev/null; then
	# We have color support; assume it's compliant with Ecma-48
	# (ISO/IEC-6429). (Lack of such support is extremely rare, and such
	# a case would tend to support setf rather than setaf.)
	color_prompt=yes
    else
	color_prompt=
    fi
fi

if [ "$color_prompt" = yes ]; then
    PS1='${debian_chroot:+($debian_chroot)}\[\033[01;32m\]\u@\h\[\033[00m\]:\[\033[01;34m\]\w\[\033[00m\]\$ '
else
    PS1='${debian_chroot:+($debian_chroot)}\u@\h:\w\$ '
fi
unset color_prompt force_color_prompt

# If this is an xterm set the title to user@host:dir
case "$TERM" in
xterm*|rxvt*)
    PS1="\[\e]0;${debian_chroot:+($debian_chroot)}\u@\h: \w\a\]$PS1"
    ;;
*)
    ;;
esac



# enable programmable completion features (you don't need to enable
# this, if it's already enabled in /etc/bash.bashrc and /etc/profile
# sources /etc/bash.bashrc).
if [ ! -z "$(shopt -p|grep shopt)" ] ; then
    if ! shopt -oq posix; then
      if [ -f /usr/share/bash-completion/bash_completion ]; then
        . /usr/share/bash-completion/bash_completion
      elif [ -f /etc/bash_completion ]; then
        . /etc/bash_completion
      fi
    fi
fi

export GIT_PS1_SHOWDIRTYSTATE=1 GIT_PS1_SHOWSTASHSTATE=1 GIT_PS1_SHOWUNTRACKEDFILES=1 GIT_PS1_SHOWUPSTREAM=verbose GIT_PS1_SHOWUPSTREAM=verbose GIT_PS1_SHOWCOLORHINTS=true
#AGM function
# disable this
# . ~/opt/bin/agm.sh
if [ -f ~/.local/lib/python2.7/site-packages/powerline/bindings/bash/powerline.sh ]; then
  source ~/.local/lib/python2.7/site-packages/powerline/bindings/bash/powerline.sh
fi

# The next line updates PATH for the Google Cloud SDK.
if [ -f ~/google-cloud-sdk/path.bash.inc ]; then
    source ~/google-cloud-sdk/path.bash.inc
fi

# The next line enables bash completion for gcloud.
if [ -f ~/google-cloud-sdk/completion.bash.inc ]; then
    source ~/google-cloud-sdk/completion.bash.inc
fi

# needed for shellcheck
if [ -f ~/.cabal/bin ] ; then
    export PATH="${PATH}:~/.cabal/bin"
fi

# sshd run moved to install.sh for manual setup
# if [ -f ~/.sshd.env ] ; then
#     # using this on chroot for chromeos
#     # sudo /usr/sbin/sshd -D & 
#     . ~/opt/bin/sshd_run.sh
# else
#     echo "no sshd at login, echo 'SSHD_LOGIN=true'> ~/.sshd.env to enable"
# fi
if [ -f ~/.ora.java.env ] ; then
    . $HOME/.ora.java.env
fi

# mounting git repos across files systems , specifically the crouton use case
export GIT_DISCOVERY_ACROSS_FILESYSTEM=1

export PATH="$PATH:$HOME/.rvm/bin" # Add RVM to PATH for scripting
export PATH="$HOME/.cargo/bin:$PATH"

# press and hold for vim
if [[ "$OSTYPE" == "darwin"* ]] ; then
    defaults write com.microsoft.VSCode ApplePressAndHoldEnabled -bool false
fi

# pyenv setup with conditional loading
[ -d "$HOME/.pyenv/bin" ] && export PATH="$HOME/.pyenv/bin:$PATH"
if command -v pyenv 1>/dev/null 2>&1; then
    export PYENV_ROOT="$HOME/.pyenv"
    
    if [[ "$EDITOR_TERMINAL" == "true" ]]; then
        # Fast loading - just add shims to PATH
        export PATH="$PYENV_ROOT/shims:$PATH"
    else
        # Full pyenv initialization
        eval "$(pyenv init -)"
    fi
fi

# rbenv setup
[ -d "$HOME/.rbenv/bin" ] && export PATH="$HOME/.rbenv/bin:$PATH"
if command -v rbenv 1>/dev/null 2>&1; then
    eval "$(rbenv init -)"
fi

# source any secrets
if [ -f ~/.secrets.env ] ; then
    source ~/.secrets.env
fi

test -f $HOME/.rbenv/shims/gh && $HOME/.rbenv/shims/gh

# /usr/local/bin
export PATH="$PATH:/usr/local/bin"

# NVM setup with conditional loading for performance
export NVM_DIR="$HOME/.nvm"
if [[ "$EDITOR_TERMINAL" == "true" ]]; then
    # Fast loading - just add node to PATH if available
    [ -d "$NVM_DIR/versions/node" ] && export PATH="$NVM_DIR/versions/node/$(ls $NVM_DIR/versions/node | tail -1)/bin:$PATH"
else
    # Full NVM initialization
    [ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"  # This loads nvm
    [ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"  # This loads nvm bash_completion
fi

if [ -f /home/linuxbrew/.linuxbrew/bin/brew ] ; then
    eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"
fi

# SF aliases load moved to daily maintenance above


# OpenClaw Integration
[ -f "$HOME/.openclaw.sh" ] && source "$HOME/.openclaw.sh"

# OpenClaw Completion
source <(openclaw completion --shell bash)

# Gemini tmux aliases
# Added by tmux-mgr
[ -f $HOME/.config/tmux-mgr/aliases.sh ] && source $HOME/.config/tmux-mgr/aliases.sh

#AGM function
#source ~/opt/bin/agm.sh
