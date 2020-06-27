#!/bin/bash
#
export BASE_DIR="$(cd "$(dirname $0)" && pwd)"
git config --global pager.branch false
git config --global push.default current

# vim
curl -fLo ~/.vim/autoload/plug.vim --create-dirs https://raw.githubusercontent.com/junegunn/vim-plug/master/plug.vim
vim +'PlugInstall --sync' +qall

[ ! -s "${HOME}/opt" ] && ln -s "${BASE_DIR}/opt" "${HOME}/opt"
for file in $(find "${BASE_DIR}/opt/profiles" -type f); do
    echo "Creating symlink to $file in home directory."
    [ ! -s "${HOME}/$(basename "${file}")" ] && ln -s "${file}" "${HOME}/$(basename "${file}")"
done

source "${HOME}/.profile"
if [ -z "$(which apt-get)" ]; then
  sudo apt-get install -y \
    jq \
    zsh
fi

# zsh 
if [ "$SHELL" != "/usr/bin/zsh" ]; then
    zsh
fi;
