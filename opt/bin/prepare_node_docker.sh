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

# prepare a node with docker installed
# * we need puppet commandline setup, along with expected modules

#
# setup the basics
apt-get update
apt-get -y install git vim curl wget python-all-dev
mkdir -p /opt/config/puppet/git
cd /opt/config/puppet/git
git config --global http.sslverify false
git clone https://review.forj.io/forj-oss/maestro
cd maestro/
bash -xe puppet/install_puppet.sh 
bash -xe puppet/install_modules.sh

#
# install docker with puppet modules
cd /opt/config/puppet/git
git config --global http.sslverify false
git clone https://review.forj.io/forj-config
cd forj-config/
