#!/bin/bash
#
function install_zsh_centos7() {
    sudo yum update -y
    sudo yum install -y git make ncurses-devel gcc autoconf man
    git clone -b zsh-5.7.1 https://github.com/zsh-users/zsh.git /tmp/zsh
    (
        cd /tmp/zsh
        ./Util/preconfig
        ./configure
        sudo make -j 20 install.bin install.modules install.fns
    )
}

export BASE_DIR="$(cd "$(dirname $0)" && pwd)"
git config --global pager.branch false
git config --global push.default current

[ ! -d "${HOME}/git" ] && mkdir -p "${HOME}/git"

# skip login check for sshd 
touch "${HOME}/.gitenv.nologin"

# vim
curl -fLo ~/.vim/autoload/plug.vim --create-dirs https://raw.githubusercontent.com/junegunn/vim-plug/master/plug.vim
vim +'PlugInstall --sync' +qall

[ ! -s "${HOME}/opt" ] && ln -s "${BASE_DIR}/opt" "${HOME}/opt"
for file in $(find "${BASE_DIR}/opt/profiles" -type f); do
    echo "Creating symlink to $file in home directory."
    [ ! -s "${HOME}/$(basename "${file}")" ] && ln -s "${file}" "${HOME}/$(basename "${file}")"
done

# force a few
for file in ".profile" ".zshrc" ".bash_logout" ".bashrc"; do
  [ -f "${HOME}/${file}" ] && rm -f "${HOME}/${file}"
  ln -s "${BASE_DIR}/opt/profiles/${file}" "${HOME}/${file}"
done 

NIX_MANAGED_FILE="${HOME}/.config/nix_managed"

if [ -f "$NIX_MANAGED_FILE" ]; then
  echo "Skipping apt-get because the env is managed with nix, found $NIX_MANAGED_FILE"
else
  if command -v apt-get &> /dev/null; then
    sudo apt-get install -y \
      corkscrew \
      htop \
      iputils-ping \
      jq \
      lsof \
      net-tools \
      psmisc \
      zsh
  fi
fi

# yum script
if [ -f "$NIX_MANAGED_FILE" ]; then
  echo "Skipping yum because the env is managed with nix, found $NIX_MANAGED_FILE"
else
  if command -v yum &> /dev/null; then
    sudo yum install -y \
      htop \
      jq \
      lsof \
      net-tools \
      psmisc \

    install_zsh_centos7
  fi
fi

# only setup these scripts when docker is installed
if command -v docker &> /dev/null; then
    if docker --version 2>&1 >/dev/null; then
        [ ! -f "${HOME}/.ruby.env" ] && source "${HOME}/opt/bin/setup_ruby-docker.sh"
        [ ! -f "${HOME}/.dindcenv" ] && source "${HOME}/opt/bin/setup_dindc_alias.sh"
    fi
fi

# don't bother installing without corkscrew
if command -v corkscrew &> /dev/null; then
    [ ! -f "${HOME}/.gitenv" ] && source "${HOME}/opt/bin/setup_git_alias.sh"
fi

if [ -f "${HOME}/.gitrepos" ] ; then
  cd "${HOME}"
  "${HOME}/.gitrepos"
fi
