#!/bin/bash
#
export BASE_DIR="$(cd "$(dirname $0)" && pwd)"
git config --global pager.branch false
git config --global push.default current

# vim
curl -fLo ~/.vim/autoload/plug.vim --create-dirs https://raw.githubusercontent.com/junegunn/vim-plug/master/plug.vim
vim +'PlugInstall --sync' +qall

ln -s "${BASE_DIR}/opt" ~/opt
source ~/.profile

# zsh 
if [ "$SHELL" != "/usr/bin/zsh" ]; then
    sudo apt install -y zsh
    zsh
fi;
