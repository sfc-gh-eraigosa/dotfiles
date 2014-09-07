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
# curl https://raw.githubusercontent.com/wenlock/myhome/master/opt/bin/prepare_node_docker.sh | sudo bash -xe
export AS_ROOT=${AS_ROOT:-0}
function DO_SUDO {
  if [ $AS_ROOT -eq 0 ] ; then
    sudo $@
  else
    $@
  fi
}

GIT_HOME=${GIT_HOME:-/opt/config/puppet/git}
REVIEW_SERVER=${GIT_HOME:-https://review.forj.io}

# prepare a node with docker installed
# * we need puppet commandline setup, along with expected modules
if [ ! $(id -u) -eq 0 ] && [ $AS_ROOT -eq 1 ] ; then
  echo "ERROR : SCRIPT should be run as sudo or root with export AS_ROOT=1"
  exit 1
fi
#
# setup the basics
DO_SUDO apt-get update
DO_SUDO apt-get -y install git vim curl wget python-all-dev
mkdir -p "$GIT_HOME"
cd "$GIT_HOME"
git config --global http.sslverify false
git clone $REVIEW_SERVER/forj-oss/maestro
cd maestro/
bash -xe puppet/install_puppet.sh 
bash -xe puppet/install_modules.sh

#
# install docker with puppet modules
cd "$GIT_HOME"
git config --global http.sslverify false
git clone $REVIEW_SERVER/p/forj-config
cd forj-config/

#
# install docker
PUPPET_MODULES=$GIT_HOME/forj-config:$GIT_HOME/maestro/puppet:/etc/puppet/modules
DO_SUDO puppet apply --modulepath=$PUPPET_MODULES -e "include docker_wrap::requirements"
