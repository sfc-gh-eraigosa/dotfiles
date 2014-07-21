# ~/.profile: executed by the command interpreter for login shells.
# This file is not read by bash(1), if ~/.bash_profile or ~/.bash_login
# exists.
# see /usr/share/doc/bash/examples/startup-files for examples.
# the files are located in the bash-doc package.

# the default umask is set in /etc/profile; for setting the umask
# for ssh logins, install and configure the libpam-umask package.
#umask 022
. /etc/environment

# if running bash
if [ -n "$BASH_VERSION" ]; then
    # include .bashrc if it exists
    if [ -f "$HOME/.bashrc" ]; then
	. "$HOME/.bashrc"
    fi
fi

# set PATH so it includes user's private bin if it exists
if [ -d "$HOME/bin" ] ; then
    PATH="$HOME/bin:$PATH"
fi
export PUPPET_MODULES=$PUPPET_MODULES:~/.puppet/modules
export PUPPET_MODULES=/opt/config/production/puppet/modules:~/git/CDK-infra/blueprints/openstack/puppet/modules:/opt/config/production/git/config/modules:/etc/puppet/modules:/home/wenlock/.puppet/modules
set -o vi
# Source git environment shortcuts
. ~/.gitenv
alias dockerup='bash ~/git/master/CDK-infra/tools/vde/openstack/docker/docker_up.sh'
alias novassh='function nova_ssh { ssh-keygen -f ~/.ssh/known_hosts -R $1;ssh -i ~/.ssh/nova-USWest-AZ3.pem -l ubuntu $1;};nova_ssh'
alias novassh1='function nova_ssh1 { ssh-keygen -f ~/.ssh/known_hosts -R $1;ssh -i ~/.ssh/nova-USWest-AZ1.pem -l ubuntu $1;};nova_ssh1'
alias sshhost='cat ~/.ssh/config|grep "Host\s"|sed "s/Host /ssh /g"'
alias irc=irssi
alias forjssh='~/bin/forjssh.sh'

force_color_prompt=yes
# Info file
id=$(hostname | awk -F. '{print $2}')
[[ `which facter` ]] && ip=$(facter ipaddress)
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
git-login
# VIMRC options
alias vimg='vim -u ~/.vimrc_green'
function vimg_set {
  rm -f ~/.vimrc_active
  ln -s ~/.vimrc_green ~/.vimrc_active;
};
alias vimw='vim -u ~/.vimrc_white'
function vimw_set {
  rm -f ~/.vimrc_active
  ln -s ~/.vimrc_white ~/.vimrc_active;
};
function vim_set {
  rm -f ~/.vimrc_active
  ln -s ~/.vimrc ~/.vimrc_active;
};
[[ ! -f ~/.vimrc_active ]] && ln -s ~/.vimrc ~/.vimrc_active
alias vi='vim -u ~/.vimrc_active'

# GIT_SAVE_OFF control functions
alias gitsave_off='export GIT_SAVE_OFF=true;echo "GIT_SAVE_OFF is true, bash_logout will not commit";'
alias gitsave='unset GIT_SAVE_OFF;echo "GIT_SAVE_OFF is unset, bash_logout will commit";'
cd ~
git pull
[[ -f ~/.custom_profile ]] && . ~/.custom_profile
