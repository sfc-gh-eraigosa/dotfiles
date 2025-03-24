#
# alias files
#


# enable color support of ls and also add handy aliases
if [ -x /usr/bin/dircolors ]; then
    test -r ~/.dircolors && eval "$(dircolors -b ~/.dircolors)" || eval "$(dircolors -b)"
    alias ls='ls --color=auto'
    #alias dir='dir --color=auto'
    #alias vdir='vdir --color=auto'

    alias grep='grep --color=auto'
    alias fgrep='fgrep --color=auto'
    alias egrep='egrep --color=auto'
fi

# some more ls aliases
alias ll='ls -alF'
alias la='ls -A'
alias l='ls -CF'

# Add an "alert" alias for long running commands.  Use like so:
#   sleep 10; alert
alias alert='notify-send --urgency=low -i "$([ $? = 0 ] && echo terminal || echo error)" "$(history|tail -n1|sed -e '\''s/^\s*[0-9]\+\s*//;s/[;&|]\s*alert$//'\'')"'

# Alias definitions.
# You may want to put all your additions into a separate file like
# ~/.bash_aliases, instead of adding them here directly.
# See /usr/share/doc/bash-doc/examples in the bash-doc package.


alias dockerup='bash ~/opt/bin/docker_up.sh'
alias novassh='function nova_ssh { ssh-keygen -f ~/.ssh/known_hosts -R $1;ssh -i ~/.ssh/nova-USWest-AZ3.pem -l ubuntu $1;};nova_ssh'
alias novassh1='function nova_ssh1 { ssh-keygen -f ~/.ssh/known_hosts -R $1;ssh -i ~/.ssh/nova-USWest-AZ1.pem -l ubuntu $1;};nova_ssh1'
alias sshhost='cat ~/.ssh/config|grep "Host\s"|sed "s/Host /ssh /g"'
alias irc=irssi
alias forjssh='~/bin/forjssh.sh'

# VIMRC options
set -o vi
alias vimg='vim -u ~/.vimrc_green'
function vimg_set {
  rm -f ~/.vimrc_active;
  ln -s ~/.vimrc_green ~/.vimrc_active;
};
alias vimw='vim -u ~/.vimrc_white'
function vimw_set {
  rm -f ~/.vimrc_active;
  ln -s ~/.vimrc_white ~/.vimrc_active;
};
function vim_set {
  rm -f ~/.vimrc_active;
  ln -s ~/.vimrc_default ~/.vimrc_active;
};
[[ ! -f ~/.vimrc_active ]] && ln -s ~/.vimrc_default ~/.vimrc_active
alias vi='vim -u ~/.vimrc_active'
alias vim='vim -u ~/.vimrc_active'

# GIT_SAVE_OFF control functions
alias gitsave_off='export GIT_SAVE_OFF=true;echo "GIT_SAVE_OFF is true, bash_logout will not commit";'
alias gitsave='unset GIT_SAVE_OFF;echo "GIT_SAVE_OFF is unset, bash_logout will commit";'

# windows manager startup
# we need to override /usr/local/startunity
if [ -f /usr/local/bin/startunity ] ; then
function startunity {
  export UBUNTU_MENUPROXY=1;
  export GTK_MODULES="unity-gtk-module";
  exec gnome-session-wrapper ubuntu;
};
fi



# setup a persistent tunnel
alias tunnelp='sudo openvpn --mktun --dev tun0'

# start gertty
# if gertty is missing, install with these commands:
# cd ~
# virtualenv gertty-env
# pip install gertty
# You can find more about gertty from https://review.openstack.org/stackforge/gertty
alias gertty='source gertty-env/bin/activate && gertty'

# setup proxy settings if a .proxy.sh exist
[ -f ~/.proxy.sh ] && . ~/.proxy.sh
alias tb=~/git/forj-oss/maestro/tools/bin/test-box.sh

# Wow aliases
alias win_nogl='LIBGL_ALWAYS_SOFTWARE=1 wine explorer'
alias win='wine explorer -opengl'
alias wow='WINEDEBUG=-all wine "/home/wenlock/.wine/drive_c/Program Files (x86)/World of Warcraft/Wow.exe"'

#
# for coros and hpchromebook 14, so we have some keys.
#
alias f11='xdotool key F11'
alias f12='xdotool key F12'
alias delkey='xdotool key Delete'

#
# Source git environment shortcuts
#

if [ -f ~/.gitenv ] ; then
    source ~/.gitenv
    if [ ! -f ~/.gitenv.nologin ]; then
        echo "running git-login, to disable execute: touch ~/.gitenv.nologin"
        fgit-login
    fi
else
    echo ".gitenv is missing, you can install with : . opt/bin/setup_git_alias.sh"
fi
# Some git shortcuts
[[ -e $(alias git-reset) ]] && unalias git-reset
alias git-reset='$HOME/opt/bin/git-reset.sh'
alias git-branches-rm='$HOME/opt/bin/git-rm-mybranches.sh'
alias git-local-master='$HOME/opt/bin/git-local-master.sh'

if [ -f ~/git/projects.cson ]; then
    [ ! -f ~/.atom/projects.cson ] && ln -s ~/git/projects.cson ~/.atom/projects.cson
fi

if [ -f $HOME/.goenv.sh ]; then
    . $HOME/.goenv.sh
fi

if [ -f $HOME/.docker.sh ]; then
    . $HOME/.docker.sh
fi

#
# Source git environment shortcuts
#
if [ -f $HOME/.dindcenv ] ; then
    source $HOME/.dindcenv
else
    echo ".dindcenv is missing, you can install with : . opt/bin/setup_dindc_alias.sh"
fi


## different git-reset
if [ -f "$HOME/.ruby.env" ]; then
  source "$HOME/.ruby.env"
fi

alias lastpass=lpass

#openvpn applescript
alias vpn="osascript -e 'tell application \"Viscosity\" to connectall'"
alias novpn="osascript -e 'tell application \"Viscosity\" to disconnectall'"

export NAMESPACE="${NAMESPACE:-default}"
alias k="kubectl --namespace=$NAMESPACE"
alias kpodjson='k get pod -o=json'
alias kpod='kpodjson|jq -r ".items[0].metadata.name"'

if [ -f ~/.custom_alias ] ; then
    . ~/.custom_alias
fi

if [ -f ~/opt/bin/tmuxinator.zsh ] ; then
    source opt/bin/tmuxinator.zsh
fi

alias ecr-login='eval $(aws ecr get-login --no-include-email)'

if command -v hub &> /dev/null; then
    eval "$(hub alias -s)"
fi

# for travis gem
[ -f "$HOME/.travis/travis.sh" ] && source "$HOME/.travis/travis.sh"

if [ -f "/usr/local/opt/nvm/nvm.sh" ] ; then
    export NVM_DIR="$HOME/.nvm"
    source "/usr/local/opt/nvm/nvm.sh"
fi
export DOCKER_STACK_ORCHESTRATOR=swarm

# setup java version
# 1.8, 11, 12, 1.7
if [ -d "/usr/libexec/java_home" ] ; then
  export JAVA_VERSION=1.8
  export JAVA_HOME=$(/usr/libexec/java_home -v ${JAVA_VERSION})
fi

if [ "$(uname -s)" = "Darwin" ]; then
    alias code="open '/Applications/Visual Studio Code.app'"
else
    alias code="/mnt/c/Program\ Files/Microsoft\ VS\ Code/Code.exe"
fi

# gpg
alias gpg-test='echo "Hello" | gpg -s'
alias gpg-list='gpg --list-secret-keys'
alias gpg-git-config='git config --global --list |egrep "(gpg|sign)"'
alias gpg-config='cat ~/.gnupg/gpg.conf'

if [ -f /usr/local/bin/dev-vpn ] ; then
    alias dev-vpn='sudo dev-vpn connect'
fi

# lets alias to python3
alias python=python3
alias pip=pip3
alias vault-login=vault-login.sh

# docker windows
if [ -d /mnt/c/Program\ Files/Docker/Docker/resources/bin/ ]; then
   alias docker='/mnt/c/Program\ Files/Docker/Docker/resources/bin/docker.exe'
   alias kubectl='/mnt/c/Program\ Files/Docker/Docker/resources/bin/kubectl.exe'
   alias docker-compose='/mnt/c/Program\ Files/Docker/Docker/resources/bin/docker-compose.exe'
fi
alias python='python3'
alias pip='pip3'

# snowsql for mac
# https://docs.snowflake.com/en/user-guide/snowsql-install-config
if [ -f /Applications/SnowSQL.app/Contents/MacOS/snowsql ]; then
    alias snowsql=/Applications/SnowSQL.app/Contents/MacOS/snowsql
fi

# install snow cli
# TODO: restore snow at some point
# snow --version || ( pip3 install --upgrade pip && python -m pip install snowflake-cli-labs )
export GOPRIVATE=github.com/snowflakedb/*
