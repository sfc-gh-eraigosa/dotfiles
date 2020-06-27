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

# if running bash
if [ -n "$BASH_VERSION" ]; then
    # include .bashrc if it exists
    if [ -f "${HOME}/.bashrc" ]; then
	. "${HOME}/.bashrc"
    fi
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
# Info file
id=$(hostname | awk -F. '{print $2}')
if [ `command -v facter` ]; then
  ip=$(facter ipaddress)
fi
echo "|$id|$ip" > ~/.info
# SSH BANNER -------------------------
rm ~/.motd
owner="wenlock"
owner=$(printf '%-40s' $owner)
host=$(hostname | awk -F. '{print $1"."$2}')
host=$(printf '%10s' $host)
echo -e "\033[1;37m┌─────────────────────────────────────────────────────────────┐" > ~/.motd
echo -e "\033[1;37m│ \033[01;31m$host \033[01;32mOWNED BY $owner\033[1;37m│" >> ~/.motd
echo -e "\033[1;37m└─────────────────────────────────────────────────────────────┘\033[00m" >> ~/.motd
cat ~/.motd

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


export PATH=~/.yarn/bin:$PATH
export PATH=~/.rbenv/shims:$PATH
export RBENV_VERSION=2.5.3

export PATH="${HOME}/.cargo/bin:$PATH"
