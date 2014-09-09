#!/bin/bash -xe

# Copyright (C) 2011-2013 OpenStack Foundation
# Copyright 2013 Hewlett-Packard Development Company, L.P.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may
# not use this file except in compliance with the License. You may obtain
# a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and limitations
# under the License.
# 
# Test install:
# curl https://raw.githubusercontent.com/wenlock/myhome/master/opt/bin/prepare_node_docker.sh | bash -xe
export DEBUG=${DEBUG:-0}
export AS_ROOT=${AS_ROOT:-0}
export SCRIPT_TEMP=$(mktemp -d)
trap 'rm -rf $SCRIPT_TEMP' EXIT

[ $DEBUG -eq 1 ] && set -x -v
function DO_SUDO {
  if [ $AS_ROOT -eq 0 ] ; then
    sudo "$@"
  else
    eval "$@"
  fi
}

function GIT_CLONE {
  [ -z $1 ] && echo "ERROR GIT_CLONE requires repo name" && exit 1
  [ -z $REVIEW_SERVER ] && echo "ERROR GIT_CLONE requires REVIEW_SERVER" && exit 1
  [ -z $GIT_HOME ] && echo "ERROR no GIT_HOME defined" && exit 1
  git config --global http.sslverify false
  [ ! -d $GIT_HOME/$1/.git ] && git clone --depth=1 $REVIEW_SERVER/p/$1 $GIT_HOME/$1
  return 0
}

GIT_HOME=${GIT_HOME:-~/prepare/git}
REVIEW_SERVER=${REVIEW_SERVER:-https://review.forj.io}

# prepare a node with docker installed
# * we need puppet commandline setup, along with expected modules
if [ ! $(id -u) -eq 0 ] && [ $AS_ROOT -eq 1 ] ; then
  echo "ERROR : SCRIPT should be run as sudo or root with export AS_ROOT=1"
  exit 1
fi
#
# setup the basics
DO_SUDO apt-get update
DO_SUDO DEBIAN_FRONTEND=noninteractive apt-get --option 'Dpkg::Options::=--force-confold' \
        --assume-yes install -y --force-yes git vim curl wget python-all-dev
mkdir -p "$GIT_HOME"

#
# clone repos
GIT_CLONE forj-config
GIT_CLONE forj-oss/maestro

DO_SUDO bash -xe $GIT_HOME/forj-oss/maestro/puppet/install_puppet.sh 
DO_SUDO bash -xe $GIT_HOME/forj-oss/maestro/puppet/install_modules.sh

#
# install docker with puppet modules
cp $GIT_HOME/forj-oss/maestro/puppet/install_modules.sh $SCRIPT_TEMP/install_modules.sh
cat > $SCRIPT_TEMP/modules.env << MODULES
unset DEFAULT_MODULES
MODULES["garethr/docker"]="0.13.0"
MODULES
DO_SUDO bash -xe $SCRIPT_TEMP/install_modules.sh
#
# install docker
PUPPET_MODULES=$GIT_HOME/forj-config/modules:$GIT_HOME/forj-oss/maestro/puppet/modules:/etc/puppet/modules
[ $DEBUG -eq 1 ] && PUPPET_DEBUG="--verbose --debug"
DO_SUDO puppet apply $PUPPET_DEBUG --modulepath=$PUPPET_MODULES -e 'include docker_wrap::requirements'

# current user should be given docker privs
CURRENT_USER=$(facter id)
[ -z $CURRENT_USER ] && echo "ERROR : failed to get current user with facter id" && exit 1
DO_SUDO puppet apply $PUPPET_DEBUG -e 'user {'"'${CURRENT_USER}'"': ensure => present, gid => "docker" }'

# build a docker image bare_precise_puppet
# This docker image should have puppet and required modules installed.
# ********** START DOCKER FILE PRECISE ****************************************
cat > $GIT_HOME/Dockerfile << DOCKER_BARE_PRECISE
# DOCKER-VERSION 0.3.4
# build a puppet based image
FROM  ubuntu:12.04
ADD . /opt/git
# Setup Minimal running system
RUN apt-get -y update; \
    apt-get -y upgrade; \
    DEBIAN_FRONTEND=noninteractive apt-get --option 'Dpkg::Options::=--force-confold' \
        --assume-yes install -y --force-yes ntpdate git vim curl wget python-all-dev;
RUN git config --global http.sslverify false
RUN bash -xe /opt/git/forj-oss/maestro/puppet/install_puppet.sh 
RUN bash -xe /opt/git/forj-oss/maestro/puppet/install_modules.sh
DOCKER_BARE_PRECISE
# ********** END DOCKER FILE PRECISE *******************************************

#
# build an image for this prepare
docker build -t ubuntu-bare-precise $GIT_HOME
if ! docker images --no-trunc | grep -e '^ubuntu-bare-precise\s.*' ; then
  echo "ERROR: ubuntu-bare-precise image not found."
  exit 1
fi

# setup beaker
DO_SUDO puppet apply $PUPPET_DEBUG \
               -e 'package {["build-essential","vim","git","make","dos2unix","libxslt-dev","libxml2-dev"]:
                              ensure => present } ->
                   package {"beaker":
                              provider => gem,
                              ensure   => latest, }'
