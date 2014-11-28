#
# alias files
#

# Source git environment shortcuts
if [ -f ~/.gitenv ] ; then
  . ~/.gitenv
else
  echo ".gitenv is missing, you can install with : . opt/bin/setup_git_alias.sh"
fi
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

git-login

# Pull the latest repos
if [ -f "${HOME}/.gitrepos" ] ; then
  cd "${HOME}"
  git pull origin $(git branch | grep '*'|awk '{print $2}')
  "${HOME}/.gitrepos"
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
