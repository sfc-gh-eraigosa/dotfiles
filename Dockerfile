#
# Easy dockerfile to test my stuff
LABEL Description="Wenlock dotfiles" Vendor="Wenlock Wizzard in a Blizzard LTD." Version="0.0.1" Maintainer="wenlock@github.com"
FROM ubuntu:bionic
WORKSPACE /dotfiles
COPY . /dotfiles
