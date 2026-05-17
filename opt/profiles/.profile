# ~/.profile: executed by the command interpreter for login shells.
# This file is not read by bash(1), if ~/.bash_profile or ~/.bash_login
# exists.
# see /usr/share/doc/bash/examples/startup-files for examples.
# the files are located in the bash-doc package.

# the default umask is set in /etc/profile; for setting the umask
# for ssh logins, install and configure the libpam-umask package.
#umask 022

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
if [ "$TERM_PROGRAM" = "vscode" ] || [ "$TERM_PROGRAM" = "cursor" ] || [ -n "$VSCODE_PID" ] || [ -n "$CURSOR_PID" ]; then
    export EDITOR_TERMINAL=true
else
    export EDITOR_TERMINAL=false
fi

# Skip expensive banner operations in editor terminals
if [ "$EDITOR_TERMINAL" = "false" ]; then
    # Info file
    id=$(hostname | awk -F. '{print $2}')
    if command -v facter >/dev/null 2>&1; then
      ip=$(facter ipaddress)
    fi
    echo "|$id|$ip" > ~/.info
    # SSH BANNER -------------------------
    if [ -f ~/.motd ]; then 
        rm ~/.motd
    fi
    owner=$(whoami)
    # Extract first two parts of hostname, avoiding trailing dots if second part is empty
    host=$(hostname | cut -d. -f1,2)
    
    # Construct the text to measure its length exactly as it will be printed
    inner_text=" $host OWNED BY $owner "
    inner_len=${#inner_text}
    
    # Generate the horizontal line using a loop (UTF-8 safe way to repeat a character)
    line=""
    i=0
    while [ $i -lt ${inner_len:-0} ]; do
        line="${line}─"
        i=$((i+1))
    done
    
    echo -e "\033[1;37m┌${line}┐" > ~/.motd
    echo -e "\033[1;37m│\033[01;31m $host \033[01;32mOWNED BY $owner \033[1;37m│" >> ~/.motd
    echo -e "\033[1;37m└${line}┘\033[00m" >> ~/.motd
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
if [ -d "${HOME}/go/shims" ] ; then
    PATH="${HOME}/go/shims:$PATH"
fi

export PATH="$PATH:${HOME}/.rvm/bin" # Add RVM to PATH for scripting
export PATH="$PATH:/usr/local/bin/docker"

[ -s "${HOME}/.rvm/scripts/rvm" ] && . "$HOME/.rvm/scripts/rvm" # Load RVM into a shell session *as a function*

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
if [ "$EDITOR_TERMINAL" = "false" ]; then
    # Start agent only if not running
    if ! pgrep -u "${USER:-$(id -un)}" ssh-agent > /dev/null 2>&1; then
      eval "$(ssh-agent -s)" > /dev/null
    fi
    export GPG_TTY=$(tty 2>/dev/null)
    # Only launch zsh if we are not already in it and it exists
    case "$-" in
        *i*)
            if [ -z "$ZSH_VERSION" ] && command -v zsh >/dev/null 2>&1; then
                exec zsh
            fi
            ;;
    esac
fi

export PATH="${KREW_ROOT:-$HOME/.krew}/bin:$PATH"
alias bazel='bazelisk'

# added by Snowflake SnowSQL installer
export PATH=${HOME}/opt/bin:$PATH

# ruby docker environment aliases
#
if [ -f ${HOME}/.ruby.env ] ; then
    source ${HOME}/.ruby.env
else
    case "$-" in
        *i*) echo ".ruby.env is missing, you can install with : . opt/scripts/docker/setup_ruby-docker.sh" ;;
    esac
fi
# Source git environment shortcuts
[ -f ${HOME}/.dindcenv ] && . ${HOME}/.dindcenv

# Load Gemini CLI environment
[ -f "$HOME/.gemini.profile" ] && . "$HOME/.gemini.profile"

# Load Nano Platform environment
[ -f "$HOME/.nano_profile" ] && . "$HOME/.nano_profile"
