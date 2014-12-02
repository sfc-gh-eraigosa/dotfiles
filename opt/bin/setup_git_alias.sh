#!/bin/bash
#vim: setlocal ft=sh:
# (c) Copyright 2014 Hewlett-Packard Development Company, L.P.
#
#    Licensed under the Apache License, Version 2.0 (the "License");
#    you may not use this file except in compliance with the License.
#    You may obtain a copy of the License at
#
#        http://www.apache.org/licenses/LICENSE-2.0
#
#    Unless required by applicable law or agreed to in writing, software
#    distributed under the License is distributed on an "AS IS" BASIS,
#    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#    See the License for the specific language governing permissions and
#    limitations under the License.


# Copyright 2012 Hewlett-Packard Development Company, L.P
#
# run this script to setup some aliases that are described in Git with Gerrit cookbook
#

# VERIFY THAT GIT IS INSTALLED
git --version 2>&1 >/dev/null
GIT_IS_AVAILABLE=$?
if [ $GIT_IS_AVAILABLE -eq 0 ]; then
  git --version
else
  echo 'ERROR: GIT is not installed'
  read -p 'Press any key to exit...'
  exit 1
fi

# VERIFY THAT CORKSCREW IS INSTALLED
which corkscrew | grep -q corkscrew
CORKSCREW_IS_AVAILABLE=$?

if ! [ $CORKSCREW_IS_AVAILABLE -eq 0 ]; then
  echo ''
  echo 'WARNING: Corkscrew is not available, corkscrew helps with proxy'
  echo ''
  echo 'Instead of setting up a tunnel with ssh, its now possible on ux and windows to setup corkscrew.'
  echo 'Corkscrew works with openssh to communicate to your local proxy, and then send communication to'
  echo 'the correct destination service you want to work with.'
  echo ''
  echo '    INSTALLATION (cygwin with cyg-get is required):'
  echo '    On windows install corkscrew with cyg-get : cyg-get corkscrew'
  echo '    On ubuntu run command : sudo apt-get install corkscrew'
  read -p 'Press any key to exit...'
fi


grep -e "\.gitenv" ~/.profile > /dev/null 2<&1
if [[ ! $? -eq 0 ]] ; then
    echo "# Source git environment shortcuts" >> ~/.profile
    echo ". ~/.gitenv" >> ~/.profile
fi
echo "#!/bin/bash" > ~/.gitenv
echo "#GIT environment shortcuts" >> ~/.gitenv
echo alias git-logout=\'ssh-add -d ~/.ssh/gerrit_keys\' >> ~/.gitenv
echo "alias git-sshkill='kill -9 \$(ps -ef|grep ssh-agent|grep -v grep | awk '\\''{printf \$2\" \" }'\'')'" >> ~/.gitenv
echo "alias git-push='git push origin HEAD:refs/for/\$(git branch -v|grep \"^\\*\"|awk '\''{printf \$2}'\'')'" >> ~/.gitenv
echo "alias git-clean='git checkout\$(git branch -v|grep \"^\\*\"|awk '\''{printf \$2}'\'');git reset --hard origin/\$(git branch -v|grep \"^\\*\"|awk '\''{printf \$2}'\'');git clean -x -d -f'" >> ~/.gitenv
cat >> ~/.gitenv << EOF
alias git-clone='fgit-clone'
alias git-tunel='fgit-tunel \$@'
alias git-testt='fgit-testt \$@'
alias git-test='fgit-test \$@'
alias git-login='fgit-login \$@'
alias git-keys='fgit-keys \$@'
alias git-config='fgit-config \$@'
alias git-addalias='fgit-addalias \$@'
alias git-projects='fgit-projects \$@'
alias git-refresh='fgit-remote-refresh'
alias git-tunelkill='fgit-tunelkill'
alias git-pull='fgit-pull'
alias git-help='fgit-help'
alias git-proxy='fgit-proxy \$@'
alias git-reset='fgit-reset'
alias git-reset-all='fgit-reset-all'
alias refresh-gitenv='refresh-gitenv'

function fgit-help()
{
  _commands=\$(alias|grep git|awk -F= '/alias/{print \$1}'|sed 's/alias //g' | awk '{printf \$1", "}' | sed 's/, \$//g')

  echo "commands available : \$_commands"
  cat << HELP_TXT

  Summary:

    git-alias's are a set of helper tools to make it easier to interact with git
    and gerrit servers.  Tired of trying to remember long worthless command lines?
    Make it simple with git alias...

    Commands will start with git-*, use git-help for instrucitons on how
    to use it.

  Available Commands:

  \$_commands

  Getting started:

  1) Configure your git client with git-config

  2) Make some gerrit keys with git-keys, we try to use clip to paste the public key
      into your buffer, you can also copy your key from ~/.ssh/gerrit_keys.pub.
      You can then paste the public key into the gerrit ssh keys interface.

  3) Load the ssh keys created from step 2 with git-login <user>, user should be
      specified if the logged on user is different than your git user.
      When using a tunnel, make sure that the gerrit username matches the tunnel
      server user name.

  4) Test your connection with git-test
      if your connection fails, it's possilbe that the gerrit ssh ports are blocked
      and you may need to use an ssh tunel.  ssh tunel allows you to communicate
      to the gerrit ssh port on standard ssh port vs high ports required by gerrit.
      Use git-tunel and git-testt to verify your connection with a tunel is working.
      When working with a tune, you should use the alias.tunel from ~/.ssh/config
      to clone your repository.

  5) Add the server alias with git-addalias -a.  When ever we add an alias we also
      create an alias for a tunel connection as well.  This alows you to work behind
      firewalls as well, as long as port 25/tcp is allowed out.

  6) Clone a repo with git-clone <alias name>:<project name>
      git-clone will also attempt to create any triggers
      git-cone will also auto setup your git-review alias for gerrit

  7) push your changes after you commit with git-push

  Detailed Help:
  {} - are optional
  <> - are required


  git-help
    Get help for git aliases.

  git-addalias <alias name> | -a
    For automatic sets up the ~/.ssh/config file for connecting with gerrit.

  git-clone <server alias>:<project name>
    Will run a git clone with the server alias specified in ~/.ssh/config.
    Setup additional aliases with git-addalias.  Use the tunel alias to connect
    to the server using the ssh tunel from git-tunel.

  git-config
    Configure git and git aliase commands.

  git-keys <email> {key_name}
    Generates the gerrit_keys into ~/.ssh
    use {key_name} to use a different name other than gerrit_keys

  git-login {user name} {pem file}
    Optionally specify a different user name or pem file to authenticate with
    different credential information other than logged user.

  git-logout
    Remove any ssh authenticated keys from current session.

  git-projects {alias} [-s]
    List gerrit projects that are available.
    -s Short description (Optional)

  git-proxy [on|off]
    when on will setup the proxy file from ~/.proxy
    when off will clear all proxy settings.

  git-push
    Push all commited changes to gerrit server using origin information from clone.

  git-refresh
    If you are switching between networks often, use this function to
    upate your git repo to use the available remote.   We'll check
    if you have http_proxy set or not after running git-proxy, or if
    you have an ssh tunel on.

  git-reset
    Start new work by reseting the git repo, cleaning it out and
    pulling the latest version of the git repo onto master.

  git-reset-all
    This should be run in the parent directory where the git repositories are located.
    Will iterate in each one directory where there is a git repository initializated
    and will reset and clear all changes, then will pull the lastest changes.

  git-sshkill
    Sometimes it's necessary to restart the ssh-agent when you login and logout
    from the shell.  This alias makes it easy to clean things up so you can run
    git-login again.

  git-test {user name}
    Perform an ssh test connection to gerrit server.  Does not use the tunnel
    connection.

  git-testt {user name}
    Perform an ssh test connection to gerrit server. Uses an ssh tunnel that
    is established with git-tunel alias.

  git-tunel {user name} {pem file}
    Optionally specify a different user name or pem file to establish a tunel
    connection on normal ssh port for gerrit server.

  git-tunelkill
    Stop ssh tunel that was created with git-tunel.

  refresh-gitenv
    Updates CDK-infra with git pull and re-executes setup_git_alias.sh

HELP_TXT

}
function get-tempfile()
{
  _temp_prefix=temp
  [ ! -z "\${1}" ] && _temp_prefix=\$1
  [ ! -z "\${2}" ] && _temp_file=\$2
  if [ -z "\${_temp_file}" ] || [ ! -f "\${_temp_file}" ] ; then
    export _temp_file=\$( mktemp -t \${_temp_prefix}.XXXXXXXXXX )
  fi
  echo -n \$_temp_file
}
function clean-tempfile()
{
  _prefix=\$1
  _skip_file=\$2
  [ ! -z "\${_skip_file}" ] && find /tmp/\$_prefix* -type f -not -name "\$(basename \$_skip_file)" | xargs rm -f {} >/dev/null 2<&1
}

# get url
function fgit-remote-url()
{
  _remote_name=origin
  [ ! -z "\${1}" ] && _remote_name=\$1
  git remote -v >/dev/null 2<&1
  if [ \$? -eq 0 ] ; then
    _remote_origin=\$(git remote -v |grep \$_remote_name|head -1|awk '{printf \$1}')
    if [[ -z "\$_remote_origin" ]]; then
#    echo "ERROR, need to be in a git repository"
#    exit 1
      echo -n
      return
    fi
  else
    echo -n
    return
  fi

  _remote_url=\$(git remote -v |grep \$_remote_name |head -1|awk '{printf \$2}')
  if [[ -z "\$_remote_url" ]]; then
#    echo "ERROR, problem getting remote url"
#    exit 2
    echo -n
    return
  fi
  echo -n \$_remote_url
}

# get alias from origin
function fgit-alias()
{
  _remote_name=origin
  [ ! -z "\${1}" ] && _remote_name=\$1
  _remote_url=\$(fgit-remote-url \$_remote_name)
  [ -z "\${_remote_url}" ] && return
  _ALIAS=\$(echo -n \$_remote_url|awk -F ':' '{printf \$1}')
  echo -n \$_ALIAS
}

# get alias from origin
function fgit-project()
{
  _remote_name=origin
  [ ! -z "\${1}" ] && _remote_name=\$1
  _remote_url=\$(fgit-remote-url \$_remote_name)
  [ -z "\${_remote_url}" ] && return
  _PROJECT=\$(echo -n \$_remote_url|awk -F ':' '{printf \$2}')
  echo -n \$_PROJECT
}

#Updates CDK-infra with git pull and re-executes setup_git_alias.sh
function refresh-gitenv()
{
# /cygdrive/c/Users/chavezom/cloned_dirs/CDK-infra
  echo -n "Enter the full path to your local CDK-infra project: "
  read CDK_INFRA_PATH

  if [[ ! -f "\$CDK_INFRA_PATH" ]] ; then
    echo "cd \$CDK_INFRA_PATH"
    cd \$CDK_INFRA_PATH
    echo "git pull"
    git pull
    echo "git reset --hard origin/master"
    git reset --hard origin/master
    echo "./setup_git_alias.sh"
    \$CDK_INFRA_PATH/vagrant/orchestrator/admin_tools/setup_git_alias.sh
    read -p "refresh-gitenv has completed succesfully, please re open a new window"
    exit 0
  else
    echo "Wrong path to CDK-infra"
    read -p "Press any key to exit..."
    exit 1
  fi
}

function fgit-getIP()
{
  _server=\$1
  ping_cmd=""
  if [ "$(uname -o)" = "Cygwin" ]; then
    pingxt=\$(echo \$(cygpath -u \$(dirname \$(echo \$COMSPEC)))/ping)
    ping_cmd="\$pingxt -n 1 -4"
    ping_str="Pinging"
  else
    ping_cmd="ping -c 1"
    ping_str="PING"
  fi

  _ip=\$(\$ping_cmd \${_server}|grep \${ping_str} | awk '{printf \$3}' | grep -oE  "[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}")

  echo \$_ip
}

function fgit-proxy()
{
  if [[ -z "\$1" ]] ; then
    _proxy_action="on"
  else
    _proxy_action=\$1
  fi

  if [[ \$_proxy_action = "on" ]] ; then
    if [[ -f ~/.proxy ]] ; then
      . ~/.proxy
      echo "set proxy on ..."
    fi
  elif [[ \$_proxy_action = "off" ]] ; then
    unset http_proxy
    unset https_proxy
    unset HTTPS_PROXY
    unset HTTP_PROXY
    echo "set proxy off ..."
  else
    echo "WARNING: incorrect argument, defaulting to on"
    [ -f ~/.proxy ] && . ~/.proxy
    echo "set proxy on ..."
  fi
}

function fgit-is-tunel-up()
{
# ps -ef|grep -e "ssh -N -L \$_ssh_port:127.0.0.1:\$_ssh_port.*\$_ssh_host" |grep -v grep >/dev/null 2<&1

  if [[ -z "\$1" ]] ; then
    echo "ERROR: we need a port number to check"
    return 1
  fi
  (echo >/dev/tcp/127.0.0.1/\$1) >/dev/null 2>&1 || return 1
  return 0
}

function fgit-isWindows()
{
  [ "\$OS" == "Windows_NT" ] && return 1
  return 0
}

function fgit-tunelkill()
{
  _temp_file=\$(fgit-ssh_info)
  _ssh_port=\$(cat \$_temp_file|awk '{printf \$2}')

  fgit-is-tunel-up \$_ssh_port
  if [[ ! \$? -eq 0 ]] ; then
    echo "Tunel is not running..."
    return
  fi

  fgit-isWindows
  if [[ $? -eq 0 ]] ; then
    _pid=\$(ps -ef|grep "ssh -N -L" | grep -v grep | awk '{printf \$2" "}')
  else
# Get-WmiObject win32_process -Filter "name='ssh.exe' and CommandLine like '%-N -L%'" | select ProcessID,Name,CommandLine
    _command="Get-WmiObject win32_process -Filter \\"name='ssh.exe' and CommandLine like '%-N -L%'\\" | select ProcessID|Format-Wide -Property ProcessID"
    _pid=\$(\$(which powershell.exe) -NoProfile -ExecutionPolicy unrestricted -Command "\$_command")
    _pid=\$(echo \$_pid |tr -d '\\n' | tr -d '\\r')
  fi

  if [[ ! -z "\$_pid" ]]; then
    kill -9 \$_pid
  fi

  fgit-is-tunel-up \$_ssh_port
  if [[ \$? -eq 0 ]] ; then
    echo "Problem closing down tunel, detected \$_ssh_port still open"
  fi
}

function fgit-pull()
{
  _remote_origin=\$(git remote -v |grep origin|head -1|awk '{printf \$1}')
  if [[ -z "\$_remote_origin" ]]; then
    echo "ERROR, need to be in a git repository"
    return
  fi
  fgit-remote-refresh
  git pull
}

function fgit-remote-refresh()
{
  _remote_origin=\$(git remote -v |grep origin|head -1|awk '{printf \$1}')
  if [[ -z "\$_remote_origin" ]]; then
    echo "ERROR, need to be in a git repository"
    return
  fi
  _ALIAS=\$(fgit-alias)
  _PROJECT=\$(fgit-project)

# remote .tunel if it exist on _ALIAS
  _ALIAS=\$(echo \$_ALIAS|sed 's/\\.tunel//g')
  _temp_file=\$(fgit-ssh_info)
  _ssh_port=\$(cat \$_temp_file|awk '{printf \$2}')
  _ssh_host=\$(cat \$_temp_file|awk '{printf \$1}')

  _current_proxy_host=\$(echo \$http_proxy|awk -F':' '{printf \$2}'|sed 's/^\\/\\///g')
  _current_proxy_port=\$(echo \$http_proxy|awk -F':' '{printf \$3}')
  if [ ! -z "\${_current_proxy_host}" ] ; then
    echo "Proxy is configured..."
    _ALIAS=\$_ALIAS.proxy
  fi

  fgit-is-tunel-up \$_ssh_port
  if [[ \$? -eq 0 ]] ; then
    echo "Detected tunel..."
    _ALIAS=\$_ALIAS.tunel
  fi

  git remote set-url \$_remote_origin \$_ALIAS:\$_PROJECT
}

function fgit-addalias()
{
  _alias=\$1
  if [ -z \$GIT_URL ] ; then
    echo "ERROR missing export GIT_URL=http://host:8080, try using git-config <email>"
    return
  fi

  if [ -z \$_alias ] ; then
    echo "ERROR usage, git-addalias <alias name> or git-addalias -a "
    echo " -a , option will automatically generate an alias name from GIT_URL"
    return
  fi

  if [[ "\$_alias" = "-a" ]] ; then
    _alias=\$(echo \$GIT_URL|sed 's/^http[s]*\\:\\/\\///g'|sed 's/^https\\:\\/\\///g'|awk -F '.' '{printf \$1}')
  fi
  _alias_tunel="\${_alias}.tunel"
  _alias_proxy="\${_alias}.proxy"

  if [[ ! -d ~/.ssh ]] ; then
    mkdir -p ~/.ssh
    chmod 700 ~/.ssh
  fi

  if [[ ! -f ~/.ssh/config ]] ; then
    touch ~/.ssh/config
  fi

  _temp_file=\$(fgit-ssh_info)
  _ssh_port=\$(cat \$_temp_file|awk '{printf \$2}')
  _ssh_server=\$(cat \$_temp_file|awk '{printf \$1}')

  _user=\$GIT_USER
  if [ "\$_user" = "" ]; then
    echo "ERROR , git-addalias requires that you set GIT_USER export or login with git-login"
    return 1
  fi

  cat ~/.ssh/config|grep "^Host "|awk '{printf \$2}' | grep "\$_alias" > /dev/null 2<&1
  s=\$?
  if [[ ! \$s -eq 0 ]] ; then
# add the alias
    echo "Host \$_alias" >> ~/.ssh/config
    echo "  User \$_user" >> ~/.ssh/config
    echo "  Hostname \$_ssh_server" >> ~/.ssh/config
    echo "  Port \$_ssh_port" >> ~/.ssh/config
    echo "  ServerAliveInterval 300" >> ~/.ssh/config
    echo "  ServerAliveCountMax 120" >> ~/.ssh/config
    echo "  identityfile ~/.ssh/gerrit_keys" >> ~/.ssh/config
    echo "git alias, \$_alias, has been added!"
# proxy on alias
    fgit-proxy "on"
    _current_proxy_host=\$(echo \$http_proxy|awk -F':' '{printf \$2}'|sed 's/^\\/\\///g')
    _current_proxy_port=\$(echo \$http_proxy|awk -F':' '{printf \$3}')
    if [ ! -z "\${_current_proxy_host}" ] ; then
      echo "Host \$_alias_proxy" >> ~/.ssh/config
      echo "  User \$_user" >> ~/.ssh/config
      echo "  Hostname \$_ssh_server" >> ~/.ssh/config
      echo "  Port \$_ssh_port" >> ~/.ssh/config
      echo "  ServerAliveInterval 300" >> ~/.ssh/config
      echo "  ServerAliveCountMax 120" >> ~/.ssh/config
      echo "  ProxyCommand \$(which corkscrew) \${_current_proxy_host} \${_current_proxy_port} 8080 %h %p" >> ~/.ssh/config
      echo "  identityfile ~/.ssh/gerrit_keys" >> ~/.ssh/config
      echo "git alias, \$_alias_proxy, has been added!"
    fi
    fgit-proxy "off"

# add the tunel alias
    echo "Host \$_alias_tunel" >> ~/.ssh/config
    echo "  User \$_user" >> ~/.ssh/config
    echo "  Hostname 127.0.0.1" >> ~/.ssh/config
    echo "  Port \$_ssh_port" >> ~/.ssh/config
    echo "  ServerAliveInterval 300" >> ~/.ssh/config
    echo "  ServerAliveCountMax 120" >> ~/.ssh/config
    echo "git alias, \$_alias_tunel, has been added!"
  else
    echo "\$_alias alias already exist in ~/.ssh/config"
    echo "If you want to update \$_alias alias follow these steps:"
    echo "  (1) Manually remove \$_alias alias from ~/.ssh/config"
    echo "  (2) Re-run git-config"
  fi

}


function fgit-createknown_host()
{
  if [ -z \$GIT_URL ] ; then
    echo "ERROR missing export GIT_URL=http://host:8080, try using git-config <email>"
    return
  fi

  _server=\$(echo \$GIT_URL|sed 's/^http[s]*\\:\\/\\///g'|awk -F':' '{printf \$1}')
  _port=\$1
  if [[ -z \$_port ]] ; then
    _port=22
  fi

  _current_user=\$(whoami)
  home_dir=\$HOME
  if [ ! -d \$home_dir ] ; then
    echo "ERROR with finding \$home_dir for known_hosts creation"
    return 1
  fi

  if [ ! -d \$home_dir/.ssh ] ; then
    mkdir -p \$home_dir/.ssh
    if [[ ! \$? -eq 0 ]] ; then
      echo "ERROR creating folder \$home_dir/.ssh"
    fi
  fi
  chmod 700 \$home_dir/.ssh
  if [[ ! \$? -eq 0 ]] ; then
    echo "ERROR changing permissions \$home_dir/.ssh"
  fi
  chown \$(id -u \$_current_user):\$(id -g \$_current_user) \$home_dir/.ssh
  if [[ ! \$? -eq 0 ]] ; then
    echo "ERROR changing ownership \$home_dir/.ssh"
  fi
  _cwd=$(pwd)
  cd \$home_dir/.ssh > /dev/null 2>&1
  if [[ ! \$? -eq 0 ]] ; then
    echo "ERROR unable to change folders to \$home_dir/.ssh"
  fi

  if [ ! -f ./known_hosts ] ; then
    echo > ./known_hosts
    if [[ ! \$? -eq 0 ]] ; then
      echo "ERROR unable to create file ./known_hosts"
    fi
  fi
  chmod 600 ./known_hosts
  if [[ ! \$? -eq 0 ]] ; then
    echo "ERROR unable to create file ./known_hosts"
  fi

  echo \$_server > ./keyhost.txt
  echo \$(fgit-getIP \$_server) >> ./keyhost.txt
  ssh-keyscan -4 -t ecdsa -f ./keyhost.txt -p \$_port  > ./tmp_key
  ((cat ./known_hosts) && ( cat ./tmp_key) )|sort -u > ./known_hosts.new

  chmod 600 ./known_hosts.new
  if [[ ! \$? -eq 0 ]] ; then
    echo "ERROR unable to set permissions ./known_hosts.new"
  fi

  chown \$(id -u \$_current_user):\$(id -g \$_current_user) ./known_hosts.new
  if [[ ! \$? -eq 0 ]] ; then
    echo "ERROR unable to set ownership ./known_hosts.new"
  fi

  mv ./known_hosts.new ./known_hosts
  if [[ ! \$? -eq 0 ]] ; then
    echo "ERROR unable to move ./known_hosts.new into place"
  fi

  grep \$_server ./known_hosts > /dev/null 2>&1
  if [[ ! \$? -eq 0 ]] ; then
    echo "ERROR failed to add \$_server to \$home_dir/.ssh/known_hosts"
    return 1
  fi

  rm ./tmp_key
  rm ./keyhost.txt
  cd $_cwd > /dev/null 2>&1
  return 0
}

function fgit-config()
{
  if [ ! -x /usr/bin/wget ] ; then
    echo "'wget' is not installed, please install 'wget' as soon as posible!."
  fi

  echo "Lets configure your git global config!!"

# set gerrit user name
  current_username=\$GIT_USER
  [ -z "\$current_username" ] && current_username=$(whoami)
  echo -n "Enter your gerrit/ux user name [\$current_username]: "
  read username

  if [ -z "\$username" ]; then
    export GIT_USER=\$current_username
    writeparam "GIT_USER" "\$current_username"
  else
    export GIT_USER=\$username
    writeparam "GIT_USER" "\$username"
  fi

# set the user.name global
  current_name=\$(git config --global user.name)
  [ -z "\$current_name" ] && current_name="First Last"
  echo -n "Enter your name for git commits [\$current_name]: "
  read name

  if [ -z "\$name" ]; then
    git config --global user.name "\$current_name"
  else
    git config --global user.name "\$name"
  fi

# set the user.email global
  current_email=\$(git config --global user.email)
  [ -z "\$current_email" ] && current_email="first.last@email.com"
  echo -n "Enter your email (must match gerrit creds) [\$current_email]: "
  read email

  if [ -z "\$email" ]; then
    git config --global user.email "\$current_email"
  else
    git config --global user.email "\$email"
  fi

# add GIT_URL to ~/.gitenv
  current_url=\$GIT_URL
  [ -z "\$current_url" ] && current_url="http://host:8080"
  echo -n "Enter gerrit server URL, export GIT_URL [\$current_url]: "
  read url

  if [ -z "\$url" ]; then
    export GIT_URL=\$current_url
    writeparam "GIT_URL" "\$current_url"
  else
    export GIT_URL=\$url
    writeparam "GIT_URL" "\$url"
  fi

# setup http proxy env
  current_proxy_host=\$PROXY_HOST
  current_proxy_port=\$PROXY_PORT

  if [ -z "\$current_proxy_host" ]; then
    if [[ ! -z "\$http_proxy" ]]; then
      current_proxy_host=\$(echo \$http_proxy|awk -F':' '{printf \$2}'|sed 's/^\\/\\///g')
      current_proxy_port=\$(echo \$http_proxy|awk -F':' '{printf \$3}')
    fi
  fi

  echo -n "Enter your proxy host [\$current_proxy_host]: "
  read proxy_host

  echo -n "Enter your proxy port number [\$current_proxy_port]: "
  read proxy_port

  if [[ ! -z "\$proxy_port" ]] ; then
    writeparam "PROXY_PORT" "\$proxy_port"
  fi

  if [[ ! -z "\$proxy_host" ]] ; then
    writeparam "PROXY_HOST" "\$proxy_host"

    echo "Saving proxy config in ~/.proxy"
    export http_proxy=http://\$proxy_host:\$proxy_port
    export https_proxy=https://\$proxy_host:\$proxy_port
    export HTTP_PROXY=http://\$proxy_host:\$proxy_port
    export HTTPS_PROXY=https://\$proxy_host:\$proxy_port

    echo "export http_proxy=\$http_proxy" > ~/.proxy
    echo "export https_proxy=\$https_proxy" >> ~/.proxy
    echo "export HTTP_PROXY=\$HTTP_PROXY" >> ~/.proxy
    echo "export HTTPS_PROXY=\$HTTPS_PROXY" >> ~/.proxy
  else
    echo "Proxy save skipped"
  fi

# some short fixes
  [ -f .git/config ] && git config core.autocrlf false

  if [[ ! -f ~/.ssh/gerrit_keys ]] ; then
    echo "Gerrit keys are not setup.  You can set them up with : git-keys <email>"
    echo "After you setup your keys, re-run git-config or run git-addalias -a"
  else
    fgit-createknown_host
    fgit-addalias -a
  fi
#TODO:  need to figure out if we use these or not
## git config --system http.sslcainfo "C:\Program Files (x86)\git\bin\curl-ca-bundle.crt"
## git config --global http.sslVerify false
}

# create a new key for gerrit authentication
function fgit-keys()
{
# email address
  _email=\$1
# name of the key
  _key_name_arg=\$2
  if [ -z \$_email ] ; then
    echo "ERROR usage: git-keys <email address>"
    return
  fi
  _key_name=gerrit_keys
  if [ ! -z \$_key_name_arg ] ; then
    _key_name=\$_key_name_arg
  fi
# create the ssh folders
  if [ ! -d ~/.ssh ] ; then
    mkdir -p ~/.ssh
    chmod 700 ~/.ssh
  fi

# gen the key
  pushd ~/.ssh > /dev/null 2<&1
  ssh-keygen -t rsa -C "\$_email" -f ./\$_key_name
  chmod 600 \$_key_name*
  popd > /dev/null 2<&1
  if [[ -f ~/.ssh/\$_key_name.pub ]] ; then
    clip < ~/.ssh/\$_key_name.pub
    if [[ $? -eq 0 ]] ; then
      echo "Keys created, ~/.ssh/\${_key_name}.pub is copied to the clip-board"
    fi
  else
    echo "Failed to create keys in ~/.ssh/\${_key_name} "
  fi
}

function fgit-login()
{
  agent_pid=\$(ps -ef | grep "ssh-agent" | grep -v "grep" | awk '{print(\$2)}')
  if [ "\$agent_pid" != "" ] ; then
    kill -9 \$agent_pid
    echo "Killing ssh-agent, process id: \$agent_pid"
  fi

  [ ! \$(ps -ef|grep ssh-agent|grep -v grep |wc -l) -gt 0 ] && eval \$(ssh-agent -s)
  _user=\$1
  _pem_file=\$2
# look for pem file in regular locations
  if [ -z \$_pem_file ] ; then
    if [ -f ~/.ssh/gerrit_keys ] ; then
      _pem_file=~/.ssh/gerrit_keys
    elif [ -f ~/.ssh/\$_user.pem ] ; then
      _pem_file=~/.ssh/\$_user.pem
    elif [ -f ~/.hpcloud/keypairs/\$_user.pem ] ; then
      _pem_file=~/.hpcloud/keypairs/\$_user.pem
    else
      echo "ERROR: no pem file found, try git-login <user> <pemfile>"
      return 1
    fi
  fi
  if [ ! -f \$_pem_file ] ; then
    echo "ERROR: unable to find pem file, try git-login <user> <pemfile>"
    return 1
  fi

  if [[ ! -z \$_user ]] ; then
    echo "setting export GIT_USER=\$_user"
    export GIT_USER=\$_user
  fi
  ssh-add \$_pem_file
  if [[ ! $? -eq 0 ]] ; then
    echo "Failed to add key, maybe agent is the problem, try running git-sshkill then this command again."
  fi
}

function fgit-ssh_info()
{
  _temp_file=\$(get-tempfile 'sshi')
  trap "clean-tempfile 'sshi' \${_temp_file}" EXIT
  echo -n "" > \$_temp_file
  wget --timeout=7 \$GIT_URL/ssh_info -O \$_temp_file --no-check-certificate >/dev/null 2<&1
  if [[ ! \$? -eq 0 ]] ; then
# retry
    if [[ ! -z \$http_proxy ]] ; then
      echo "retrying connection with proxy on..."
      fgit-proxy "on"
      wget --timeout=10 \$GIT_URL/ssh_info -O \$_temp_file --no-check-certificate >/dev/null 2<&1
      if [[ ! \$? -eq 0 ]] ; then
        echo "FAILED to get ssh_info from \$GIT_URL/ssh_info"
        return 1
      fi
    else
      echo "retrying connection with proxy off..."
      fgit-proxy "off"
      wget --timeout=10 \$GIT_URL/ssh_info -O \$_temp_file --no-check-certificate >/dev/null 2<&1
      if [[ ! \$? -eq 0 ]] ; then
        echo "FAILED to get ssh_info from \$GIT_URL/ssh_info"
        return 1
      fi
    fi
  fi
  echo -n \$_temp_file
}

function fgit-ssh_connect()
{
  _temp_file=\$(fgit-ssh_info)
  [ ! \$? -eq 0 ] && return
  _ssh_port=\$(cat \$_temp_file|awk '{printf \$2}'| tr -d "\n" | tr -d "\r")
  _ssh_server=\$(cat \$_temp_file|awk '{printf \$1}'| tr -d "\n" | tr -d "\r")
  _userlogin=\$(whoami)
  if [ ! -z "\$GIT_USER" ]; then
    _userlogin=\$GIT_USER
  fi
  fgit-is-tunel-up \$_ssh_port
  if [[ \$? -eq 0 ]] ; then
    _ssh_server=127.0.0.1
  fi

  _ssh_connect=\$(echo -n "\$_userlogin@\$_ssh_server -p \$_ssh_port"| tr -d "\n" | tr -d "\r")
  _ALIAS=\$(fgit-alias)
  [ ! -z "\${_ALIAS}" ] && _ssh_connect=\$(echo -n \$_ALIAS | tr -d "\n" | tr -d "\r")

  if [ ! -z "\$1" ] ; then
    _ssh_connect=$(echo -n \$1 | tr -d "\n" | tr -d "\r")
  fi

  _clean_ssh_connect=\$(echo -n \$_ssh_connect| tr -d "\n" | tr -d "\r")
  echo -n \$_clean_ssh_connect
}

function fgit-gerritcmd()
{
  _command=\$@
  if [[ -z "\$_command" ]] ; then
    echo "FAILED to execute gerrit command, missing <command> agrument"
    exit 1
  fi

  results=\$(ssh \$_command  2>&1 |xargs -i printf {}';')
  echo \$results|grep 'Permission denied' >/dev/null 2<&1
  if [[ \$? -eq 0 ]] ; then
    git-sshkill
    fgit-login
    results=\$(ssh \$_userlogin@\$_ssh_server -p \$_ssh_port \$_command 2>&1 |xargs -i printf {}';')
  fi
  proj_log=\$(get-tempfile 'proj')
  trap "clean-tempfile 'proj' \${proj_log}" EXIT
  echo "\$results"|sed 's/;$//'| sed 's/;/\n/g' > \$proj_log
  echo -n \$proj_log
}

function fgit-projects()
{
  if [ -z \$GIT_URL ] ; then
    echo "ERROR missing export GIT_URL=http://host:8080"
    return
  fi

  _alias=
  [ ! -z "\$1" ] && [ ! "\$1" = "-s" ] && _alias=\$1
  _connect=\$(fgit-ssh_connect \$_alias)
  proj_log=\$(fgit-gerritcmd \$_connect gerrit ls-projects)
  _connect=\$(echo -n \$_connect|sed 's/ -p /:/g')

  while read -r line
  do
    if [ -n "\$1" ] && [ "\$1" = "-s" ]
    then
      echo "\$line"
    else
      if [[ \$_connect == *@*:* ]] ; then
        echo "git-clone ssh://\$_connect/\$line"
      else
        echo "git-clone \$_connect:\$line"
      fi
    fi
  done < <(grep '' \$proj_log)
}

#depricate
function fgit-testt()
{
  if [ -z \$GIT_URL ] ; then
    echo "ERROR missing export GIT_URL=http://host:8080"
    return
  fi

  _temp_file=\$(fgit-ssh_info)
  _ssh_port=\$(cat \$_temp_file|awk '{printf \$2}')
  _user=\$(whoami)

  if [ ! -z "\$1" ]; then
    _user=\$1
  fi

  if [ ! -z "\$GIT_USER" ]; then
    _user=\$GIT_USER
  fi

  if [[ -z "\$_user" ]] ; then
    echo "ERROR: we need a user name, try git-login <user> or git-test <user>"
    return
  fi

  fgit-createknown_host \$_ssh_port
  echo "Testing connection to 127.0.0.1 on port \$_ssh_port ..."
  eval ssh -T \$_user@127.0.0.1 -p \$_ssh_port
}

function fgit-test()
{
  if [ -z \$GIT_URL ] ; then
    echo "ERROR missing export GIT_URL=http://host:8080, try using git-config <email>"
    return
  fi

  _temp_file=\$(fgit-ssh_info)
  _ssh_port=\$(cat \$_temp_file|awk '{printf \$2}')
  _ssh_server=\$(cat \$_temp_file|awk '{printf \$1}')
  _user=\$(whoami)

  if [ ! -z "\$1" ]; then
    _user=\$1
  fi

  if [ ! -z "\$GIT_USER" ]; then
    _user=\$GIT_USER
  fi

  if [[ -z "\$_user" ]] ; then
    echo "ERROR: we need a user name, try git-login <user> or git-test <user>"
    return
  fi

  fgit-is-tunel-up \$_ssh_port
  if [[ \$? -eq 0 ]] ; then
    echo "Testing connection to 127.0.0.1 on port \$_ssh_port ..."
    eval ssh -T \$_user@127.0.0.1 -p \$_ssh_port
  else
    echo "Testing connection to \$_ssh_server ..."
    eval ssh -T \$_user@\$_ssh_server -p \$_ssh_port
  fi
}

function fgit-tunel()
{
  _user=\$1
  _pem_file=\$2
  if [ -z \$GIT_URL ] ; then
    echo "ERROR missing export GIT_URL=http://host:8080"
    return
  fi

# note here the tunel user and git user need to be the same name!!
  if [ ! -z "\$GIT_USER" ]; then
    _user=\$GIT_USER
  fi

  if [ -z \$_user ] ; then
    echo "ERROR usage: git-tunel <user>"
    return
  fi


# get ssh port
  _temp_file=\$(fgit-ssh_info)

  if [[ -z "\$_ssh_port" ]] ; then
    echo "Failed to get ssh port from \$GIT_URL"
    return
  fi

  fgit-is-tunel-up \$_ssh_port
  if [[ \$? -eq 0 ]];then
    echo "ERROR: there is already a process listening on port \$_ssh_port, try git-tunelkill or lsof -i:\$_ssh_port to kill the current listening process"
    return
  fi

# look for pem file in regular locations
  if [ -z \$_pem_file ] ; then
    if [ -f ~/.ssh/\$_user.pem ] ; then
      _pem_file=~/.ssh/\$_user.pem
    elif [ -f ~/.hpcloud/keypairs/\$_user.pem ] ; then
      _pem_file=~/.hpcloud/keypairs/\$_user.pem
    else
      echo "ERROR: no pem file found, try git-tunel \$_user <pemfile>"
      return
    fi
  fi

# get server
  _ssh_host=\$(cat \$_temp_file|awk '{printf \$1}')

  if [[ -z "\$_ssh_host" ]] ; then
    echo "Failed to get ssh host from \$GIT_URL"
    return
  fi

# spawn tunnel
  echo "Spawning ssh tunnel to \$_ssh_host on port \$_ssh_port"
  ssh -N -L \$_ssh_port:127.0.0.1:\$_ssh_port -i \$_pem_file \$_user@\$_ssh_host &
  if [[ $? -eq 0 ]] ; then
    echo "Tunel created!  use git-testt and tunel alias to connect"
    echo "  Use git-refresh to update your origin to the tunel url."
  fi
}

function fgit-clone()
{
#check for url pattern
  if [[ \$_connect == *://*@*:*/* ]] ; then
    _ALIAS=\$(echo \$1|awk -F'/' '{print \$1."/"\$2."/"\$3}')
    _PROJECT=\$(echo \$1|awk -F'/' '{print \$4}')
  else
    _ALIAS=\$(echo \$1|awk -F ':' '{printf \$1}')
    _PROJECT=\$(echo \$1|awk -F ':' '{printf \$2}')
  fi
  if [ -z \$GIT_URL ] ; then
    echo "ERROR: git-clone requires \\\$GIT_URL to be set"
    return
  fi
  if [ -z \$_PROJECT ] ; then
    echo "ERROR: Specify a project name git-clone alias:projectname"
    return
  fi
  if [ -z \$_ALIAS ] ; then
    echo "ERROR: Specify a alias name git-clone alias:projectname"
    return
  fi

  _temp_file=\$(fgit-ssh_info)
  _ssh_port=\$(cat \$_temp_file|awk '{printf \$2}')
  _ssh_host=\$(cat \$_temp_file|awk '{printf \$1}')

  fgit-is-tunel-up \$_ssh_port
  if [[ \$? -eq 0 ]] ; then
    echo "Detected tunel..."
    _ALIAS=\$_ALIAS.tunel
  fi

  echo "Cloning project \$_PROJECT on alias \$_ALIAS"
  git clone \$_ALIAS:\$_PROJECT

#pushd to last folder when project has slashes
  _cwd=\$(pwd)
  cd \$(printf '%s\n' "\${_PROJECT##*/}")
  wget -O .git/hooks/commit-msg \$GIT_URL/tools/hooks/commit-msg --no-check-certificate
  chmod u+x .git/hooks/commit-msg
  [ -f .git/config ] && git config core.autocrlf false

  echo "Adding git-review remote"
  git remote add gerrit ssh://\$_ALIAS:\$_ssh_port/\$_PROJECT
  git config core.autocrlf false
  cd \$_cwd
}

#Example: readparam "GIT_USER"
readparam()
{
  FILE=~/git.properties
  R=""
  if [ -f "\$FILE" ]  ; then
    VALUE=\$(grep -E "^ *\$1" \$FILE  | cut -f2 -d'=')
    [ -n "\$VALUE" ] && R="\$VALUE"
  fi
  echo "\$R";
}

#Example: writeparam "GIT_USER" "chavezom"
writeparam()
{
  FILE=~/git.properties

  if [ ! -f \$FILE ]  ; then
    touch \$FILE
  fi

  grep -E "^ *\$1=" \$FILE 2>&1 >/dev/null
  PARAM_EXISTS=\$?

  if [ \$PARAM_EXISTS -eq 0 ]; then
    cat \$FILE |grep -v -e "^\$1" > "\$FILE.bak"
    mv "\$FILE.bak" "\$FILE"
    echo "\$1=\$2" >> \$FILE
  else
    echo "\$1=\$2" >> \$FILE
  fi
}

# reset a git repository to it's origin
function fgit-reset()
{
  _CWD=\$(pwd)
  if git rev-parse --show-toplevel >/dev/null 2<&1 ; then
    _GIT_REPO_ROOT=\$(git rev-parse --show-toplevel)
    if [ "\${_GIT_REPO_ROOT}" = "\$HOME" ]; then
      echo "WARNING: you are about to reset your home folder!!! [\$_GIT_REPO_ROOT] "
      echo -n "Do you really want to continue, if so type: [Yes] : "
      read answer
    else
      answer="Yes"
    fi

    if [ "\${answer}" = "Yes" ]; then
      cd "\${_GIT_REPO_ROOT}"
      _BRANCH=\$(git branch|grep '^\* '|sed 's/^\*\s//g')
      git reset --hard origin/master; git clean -x -d -f; git pull origin \${_BRANCH};
    else
      echo "Skipping reset ..."
    fi
  fi
  cd "\${_CWD}"
}
function fgit-reset-all()
{
  find . -maxdepth 1 -mindepth 1 -type d  -printf "%f\n"|xargs -i bash -c 'cd {};git rev-parse --show-toplevel 2>/dev/null;if [ $? -eq 0 ]; then git reset --hard;git clean -x -d -f;git pull origin stable; else echo "{} is not a git repository"; fi;';
}

[ "\$(readparam 'GIT_USER')" != "" ] && export GIT_USER=\$(readparam "GIT_USER")
[ "\$(readparam 'GIT_URL')" != "" ] && export GIT_URL=\$(readparam "GIT_URL")
[ "\$(readparam 'PROXY_HOST')" != "" ] && export PROXY_HOST=\$(readparam "PROXY_HOST")
[ "\$(readparam 'PROXY_PORT')" != "" ] && export PROXY_PORT=\$(readparam "PROXY_PORT")

if [ -f "~/.proxy" ]  ; then
    . ~/.proxy
fi

EOF
# disable till we find a workaround to bug in vagrant
echo "if [[ ! \"\$(whoami)\" = \"vagrant\" ]] ; then">>~/.gitenv
echo "echo \"commands available : \$(alias|grep git|awk -F= '/alias/{print \$1}'|sed 's/alias //g' | awk '{printf \$1\", \"}')\" |sed 's/, \$//g'">>~/.gitenv
echo "fi" >>~/.gitenv
chmod 755 ~/.gitenv
. ~/.gitenv
