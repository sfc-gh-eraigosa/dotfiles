# shellcheck shell=bash
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
# shellcheck disable=SC2142 # novassh and novassh1 are intentional wrapper aliases around functions
alias novassh='function nova_ssh { ssh-keygen -f ~/.ssh/known_hosts -R $1;ssh -i ~/.ssh/nova-USWest-AZ3.pem -l ubuntu $1;};nova_ssh'
# shellcheck disable=SC2142
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

# Source hardware detection and skip unsupported tools
if [ -f ~/opt/lib/hardware.sh ]; then
  . ~/opt/lib/hardware.sh
  if [ -f ~/opt/lib/unsupported_tools.sh ]; then
    . ~/opt/lib/unsupported_tools.sh
  fi
fi

# Wow aliases
alias win_nogl='LIBGL_ALWAYS_SOFTWARE=1 wine explorer'
alias win='wine explorer -opengl'
alias wow='WINEDEBUG=-all wine "${HOME}/.wine/drive_c/Program Files (x86)/World of Warcraft/Wow.exe"'

#
# for coros and hpchromebook 14, so we have some keys.
#
alias f11='xdotool key F11'
alias f12='xdotool key F12'
alias delkey='xdotool key Delete'

#
# Git shortcuts (git-reset, git-reset-all, git-clean, git-help) — the slim
# replacement for the retired Gerrit-era ~/.gitenv generator. No startup side
# effects; safe to source unconditionally.
#
if [ -f "$HOME/.gitools.sh" ] ; then
    . "$HOME/.gitools.sh"
fi
# Some git shortcuts
alias git-branches-rm='$HOME/opt/scripts/git/git-rm-mybranches.sh'
alias git-local-master='$HOME/opt/scripts/git/git-local-master.sh'

if [ -f ~/git/projects.cson ]; then
    [ ! -f ~/.atom/projects.cson ] && ln -s ~/git/projects.cson ~/.atom/projects.cson
fi

# if [ -f $HOME/.goenv.sh ]; then
#     . $HOME/.goenv.sh
# fi

if [ -f $HOME/.docker.sh ]; then
    . $HOME/.docker.sh
fi

#
# Source git environment shortcuts
#
if [ -f $HOME/.dindcenv ] ; then
    source $HOME/.dindcenv
else
    echo ".dindcenv is missing, you can install with : . opt/scripts/docker/setup_dindc_alias.sh"
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
alias k='kubectl --namespace=$NAMESPACE'
alias kpodjson='k get pod -o=json'
alias kpod='kpodjson|jq -r ".items[0].metadata.name"'

if [ -f ~/.custom_alias ] ; then
    . ~/.custom_alias
fi

if [ -f ~/opt/bin/tmuxinator.zsh ] ; then
    source ~/opt/bin/tmuxinator.zsh
fi

alias ecr-login='eval $(aws ecr get-login --no-include-email)'

if command -v hub &> /dev/null; then
    eval "$(hub alias -s)"
fi

# for travis gem
[ -f "$HOME/.travis/travis.sh" ] && source "$HOME/.travis/travis.sh"

# if [ -f "/usr/local/opt/nvm/nvm.sh" ] ; then
#     export NVM_DIR="$HOME/.nvm"
#     source "/usr/local/opt/nvm/nvm.sh"
# fi
export DOCKER_STACK_ORCHESTRATOR=swarm

# setup java version
# 1.8, 11, 12, 1.7
if [ -d "/usr/libexec/java_home" ] ; then
  export JAVA_VERSION=1.8
  JAVA_HOME=$(/usr/libexec/java_home -v "${JAVA_VERSION}"); export JAVA_HOME
fi

# Windows paths (WSL): prefer the install-time cache written by
# opt/bin/install_windows.sh (resolved from the real Windows env via
# wslpath, so a custom automount root or relocated ProgramFiles is honored);
# fall back to the standard /mnt/c layout so a missing cache never breaks login.
# shellcheck disable=SC1091
[ -f "${HOME}/.cache/dotfiles/winenv.sh" ] && . "${HOME}/.cache/dotfiles/winenv.sh"
WIN_PROGRAM_FILES="${WIN_PROGRAM_FILES:-/mnt/c/Program Files}"

# shellcheck disable=SC2139  # intentional: bake the install-time-resolved path into the alias
if [ "$(uname -s)" = "Darwin" ]; then
    alias code="open '/Applications/Visual Studio Code.app'"
else
    alias code="\"${WIN_PROGRAM_FILES}/Microsoft VS Code/Code.exe\""
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

# docker windows (WSL): Docker Desktop's Windows CLIs are a fallback only.
# Never shadow a working Linux CLI, and only alias the .exe when Windows-exe
# interop actually works — the WSLInterop binfmt registration can get wiped
# (systemd/WSL race), and then every .exe fails with "exec format error".
# Note: Docker Desktop's /usr/bin/docker shim dangles when Desktop isn't
# running, so test executability, not just presence.
# shellcheck disable=SC2139  # intentional: bake the install-time-resolved path into the aliases
if [ -d "${WIN_PROGRAM_FILES}/Docker/Docker/resources/bin/" ]; then
   if grep -qs '^enabled' /proc/sys/fs/binfmt_misc/WSLInterop /proc/sys/fs/binfmt_misc/WSLInterop-late; then
      [ -x "$(command -v docker 2>/dev/null)" ]         || alias docker="\"${WIN_PROGRAM_FILES}/Docker/Docker/resources/bin/docker.exe\""
      [ -x "$(command -v kubectl 2>/dev/null)" ]        || alias kubectl="\"${WIN_PROGRAM_FILES}/Docker/Docker/resources/bin/kubectl.exe\""
      [ -x "$(command -v docker-compose 2>/dev/null)" ] || alias docker-compose="\"${WIN_PROGRAM_FILES}/Docker/Docker/resources/bin/docker-compose.exe\""
   elif ! [ -x "$(command -v docker 2>/dev/null)" ]; then
      # No working Linux docker AND no Windows-exe interop: fail loud and clear
      # instead of "exec format error" / "no such file or directory".
      docker() {
         echo "docker: Docker Desktop isn't running (its /usr/bin/docker shim is dangling)" >&2
         echo "        and Windows-exe interop is unavailable (WSLInterop binfmt not registered)." >&2
         echo "Fix:    start Docker Desktop on Windows, or restore interop with:" >&2
         echo "        sudo sh -c 'echo \":WSLInterop:M::MZ::/init:PF\" > /proc/sys/fs/binfmt_misc/register'" >&2
         return 127
      }
   fi
fi

# snowsql for mac
# https://docs.snowflake.com/en/user-guide/snowsql-install-config
if [ -f /Applications/SnowSQL.app/Contents/MacOS/snowsql ]; then
    alias snowsql=/Applications/SnowSQL.app/Contents/MacOS/snowsql
fi
if [ -f ~/opt/bin/snowsql ]; then
    alias snowsql=~/opt/bin/snowsql
fi
alias cursor='/Applications/Cursor.app/Contents/MacOS/Cursor'

function sfhelp() {
    echo "Help with sf ws commands:"
    cat <<'EOF'
    sflist - list all workspaces, alias for "sf ws ls"

    sfssh - ssh to the workspace, alias for "sf ws ssh gco2"

    sfcreate - create a new workspace, alias for "sf ws create --os rocky9 --name gco2 --customization off"
       usage: sfcreate [-nc] [-nr] [-s] [<name>]
         -nc - no customization, default is customization on
         -nr - no rocky9, default is rocky9
         -s  - small workspace (1 unit, for directed tests / non-monorepo work)
         <name> - name of the workspace, default is gco2

    sfcode - code editor for the workspace, alias for "sf ws code gco2 --file <project-file> --ide cursor"
      -f - project file name
      -i - ide to use
     
EOF
}

function sfcode() {
    local NAME="gco2"
    local PROJECT_FILE; PROJECT_FILE="$HOME/$(whoami).code-workspace"
    local IDE="cursor"
    
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -h)
                sfhelp
                return 0
                ;;
            -f)
                PROJECT_FILE="$2"
                shift 2
                ;;
            -i)
                IDE="$2"
                shift 2
                ;;
            *)
                NAME="$1"
                shift
                ;;
        esac
    done
    
    echo "sync credentials..."
    sf auth login --login-services optional-service-credentials > /dev/null
    echo "sf ws code ${NAME} --file ${PROJECT_FILE} --ide ${IDE}"
    eval sf ws code ${NAME} --file ${PROJECT_FILE} --ide ${IDE}
}

function sfcreate() {
    if [[ "${DEBUG}" = "true" ]]; then
        set -x -v;
    fi
    local NAME="gco2"
    local NO_CUSTOMIZATION=
    local ROCKY9="--os rocky9"
    local INSTANCE_PROFILE=

    while [[ $# -gt 0 ]]; do
        case "$1" in
            -h)
                sfhelp
                return 0
                ;;
            -nr)
                ROCKY9=""
                shift
                ;;
            -nc)
                NO_CUSTOMIZATION="--customization off"
                shift
                ;;
            -s)
                INSTANCE_PROFILE="--instance-profile small"
                shift
                ;;
            *)
                NAME="$1"
                shift
                ;;
        esac
    done
    echo "using name = ${NAME}"

    echo "sf ws create --name ${NAME} ${NO_CUSTOMIZATION} ${ROCKY9} ${INSTANCE_PROFILE}"
    eval sf ws create --name ${NAME} ${NO_CUSTOMIZATION} ${ROCKY9} ${INSTANCE_PROFILE}
    eval sf ws ssh ${NAME}
}

alias sfssh='sf ws ssh'
alias sfls='sf ws ls'

function tmux4() {
    tmux new-session -s dev \; \
        split-window -h \; \
        split-window -v \; \
        select-pane -t 0 \; \
        split-window -v \; \
        select-layout tiled \; \
        attach
}
alias tdev='tmux attach -t dev'
gorun() { local f; f=$(mktemp -t gorun-XXXX).go; cat >"$f"; go run "$f"; rm "$f"; }
alias avalanche_up='GODEBUG="x509ignoreCN=0" go run ./cmd/avaServer -yes-i-really-want-to-disable-authentication -mig-bypass-sha256 -overridedb 127.0.0.1'
if command -v sf &> /dev/null; then
    eval "$(sf aliases)"
fi

if [ -f ~/opt/bin/locales.sh ]; then
    source ~/opt/bin/locales.sh
fi
alias wifi-manage='~/git/dotfiles/opt/scripts/system/wifi-manage.sh'
