# ~/.profile: executed by the command interpreter for login shells.
# This file is not read by bash(1), if ~/.bash_profile or ~/.bash_login
# exists.
# see /usr/share/doc/bash/examples/startup-files for examples.
# the files are located in the bash-doc package.

# the default umask is set in /etc/profile; for setting the umask
# for ssh logins, install and configure the libpam-umask package.
#umask 022
autoload -Uz compinit
compinit

if [ ! -z "${GREP_OPTIONS}" ]; then
  alias grep="grep ${GREP_OPTIONS}"
  unset GREP_OPTIONS
fi

if [ -f /etc/environment ] ; then
  . /etc/environment
fi

# set PATH so it includes user's private bin if it exists
if [ -d "${HOME}/bin" ] ; then
    PATH="${HOME}/bin:$PATH"
fi
if [ -d "${HOME}/opt/bin" ] ; then
    PATH="${HOME}/opt/bin:$PATH"
fi

# cabal
if [ -d "${HOME}/.cabal/bin" ] ; then
      PATH="${HOME}/.cabal/bin:$PATH"
fi

force_color_prompt=yes
# Detect if we're in VSCode/Cursor terminal
if [[ "$TERM_PROGRAM" == "vscode" ]] || [[ "$TERM_PROGRAM" == "cursor" ]] || [[ -n "$VSCODE_PID" ]] || [[ -n "$CURSOR_PID" ]]; then
    export EDITOR_TERMINAL=true
else
    export EDITOR_TERMINAL=false
fi

# Skip expensive banner operations in editor terminals
if [[ "$EDITOR_TERMINAL" == "false" ]]; then
    # Info file
    id=$(hostname | awk -F. '{print $2}')
    if [ `command -v facter` ]; then
      ip=$(facter ipaddress)
    fi
    echo "|$id|$ip" > ~/.info
    # SSH BANNER -------------------------
    if [ -f ~/.motd ]; then 
        rm ~/.motd
    fi
    owner="wenlock"
    owner=$(printf '%-40s' $owner)
    host=$(hostname | awk -F. '{print $1"."$2}')
    host=$(printf '%10s' $host)
    echo -e "\033[1;37m┌─────────────────────────────────────────────────────────────┐" > ~/.motd
    echo -e "\033[1;37m│ \033[01;31m$host \033[01;32mOWNED BY $owner\033[1;37m│" >> ~/.motd
    echo -e "\033[1;37m└─────────────────────────────────────────────────────────────┘\033[00m" >> ~/.motd
    cat ~/.motd
fi

if [ -f "${HOME}/.custom_profile" ] ; then
  . "${HOME}/.custom_profile"
fi

# powerline setup
if [ -d "${HOME}/.local/bin" ] ; then
  PATH="${HOME}/.local/bin:$PATH"
fi
if [ ! -d "${HOME}/.fonts/" ] ; then
  mkdir -p "${HOME}/.fonts/"
fi
if [ ! -d "${HOME}/.config/fontconfig/conf.d/" ] ; then
  mkdir -p "${HOME}/.config/fontconfig/conf.d/"
fi
if [ -f "${HOME}/git/powerline/font/PowerlineSymbols.otf" ] ; then
  cp "${HOME}/git/powerline/font/PowerlineSymbols.otf" "${HOME}/.fonts/"
fi
if [ -f "${HOME}/.config/fontconfig/conf.d/10-powerline-symbols.conf" ] ; then
  cp "${HOME}/git/powerline/font/10-powerline-symbols.conf" "${HOME}/.config/fontconfig/conf.d/10-powerline-symbols.conf"
fi
if [ ! -d "${HOME}/go" ] ; then
    mkdir "${HOME}/go"
fi

export PATH="$PATH:${HOME}/.rvm/bin" # Add RVM to PATH for scripting
export PATH="$PATH:/usr/local/bin/docker"

[[ -s "${HOME}/.rvm/scripts/rvm" ]] && source "$HOME/.rvm/scripts/rvm" # Load RVM into a shell session *as a function*
# Source git environment shortcuts
[[ -f "${HOME}/.dindcenv" ]] && . "${HOME}/.dindcenv"

#
# ruby docker environment aliases
#
if [ -f "${HOME}/.ruby.env" ] ; then
    source "${HOME}/.ruby.env"
else
    echo ".ruby.env is missing, you can install with : . opt/bin/setup_ruby-docker.sh"
fi

#
# iterm integration with brew install https://gist.github.com/ZenLulz/c812f70fc86ebdbb189d9fb82f98197e
#  brew cask install iterm2
#test -e "${HOME}/.iterm2_shell_integration.bash" && source "${HOME}/.iterm2_shell_integration.bash"


export PATH=$HOME/.yarn/bin:$PATH
export PATH=$HOME/.rbenv/shims:$PATH
export PATH=$HOME/.nodenv/shims:$PATH
export PATH=$HOME/.cargo/shims:$PATH
export RBENV_VERSION=2.7.1

test -e "/workspace/Human-Connection/.devcontainer/profile_devcontainer_alias.sh" && \
   source "/workspace/Human-Connection/.devcontainer/profile_devcontainer_alias.sh"

if [ -d "/home/linuxbrew" ]; then
    eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"
fi
# docker windows
if [ -d "/mnt/c/Program\ Files/Docker/Docker" ]; then
    alias docker='/mnt/c/Program\ Files/Docker/Docker/resources/bin/docker.exe'
    alias docker-compose='/mnt/c/Program\ Files/Docker/Docker/resources/bin/docker-compose.exe'
fi

# pyenv, rbenv, goenv paths
[ -d "$HOME/.pyenv/bin" ] && export PATH="$PATH:$HOME/.pyenv/bin"
[ -d "$HOME/.rbenv/bin" ] && export PATH="$PATH:$HOME/.rbenv/bin"
[ -d "$HOME/.goenv/bin" ] && export PATH="$PATH:$HOME/.goenv/bin"

# Skip SSH agent and shell launching in editor terminals
if [[ "$EDITOR_TERMINAL" == "false" ]]; then
    # Start agent only if not running
    if ! pgrep -u "$USER" ssh-agent > /dev/null; then
      eval "$(ssh-agent -s)" > /dev/null
    fi
    export GPG_TTY=$(tty)
    # Only launch zsh if we are not already in it and it exists
    if [ -z "$ZSH_VERSION" ] && command -v zsh &>/dev/null; then
        exec zsh
    fi
fi

export PATH="${KREW_ROOT:-$HOME/.krew}/bin:$PATH"
alias bazel='bazelisk'

# added by Snowflake SnowSQL installer
export PATH=/Users/eraigosa/opt/bin:$PATH
