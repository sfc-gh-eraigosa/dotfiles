#
# Easy dockerfile to test my stuff
FROM ubuntu:bionic
LABEL Description="Wenlock dotfiles" Vendor="Wenlock Wizzard in a Blizzard LTD." Version="0.0.1" Maintainer="wenlock@github.com"
WORKDIR /dotfiles
COPY . /dotfiles
RUN ls -altr
