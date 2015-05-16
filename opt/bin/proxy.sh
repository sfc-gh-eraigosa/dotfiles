#!/bin/bash
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
# relies on environment option PROXY being configured
#TODO: need to check all interfaces
ETH0_IP=""
WLAN_IP=""
TUN0_IP=""
if ifconfig|grep 'Link encap'|awk '{print $1}'|grep eth0 > /dev/null ; then
    ETH0_IP=$(ifconfig eth0|grep 'inet addr'|awk -F: '{print $2}'|awk '{print $1}')
fi
if ifconfig|grep 'Link encap'|awk '{print $1}'|grep wlan0 > /dev/null ; then
    WLAN_IP=$(ifconfig wlan0|grep 'inet addr'|awk -F: '{print $2}'|awk '{print $1}')
fi
if ifconfig|grep 'Link encap'|awk '{print $1}'|grep tun0 > /dev/null ; then
    TUN0_IP=$(ifconfig tun0|grep 'inet addr'|awk -F: '{print $2}'|awk '{print $1}')
fi
if [ "${ETH0_IP}" = "" ] ; then
    GET_IP_CLI=$WLAN_IP
else
    GET_IP_CLI=$ETH0_IP
fi
if [ ! -z "${TUN0_IP}" ] ; then
    GET_IP_CLI=$TUN0_IP
fi
[ "${GET_IP_CLI}" = "" ] && GET_IP_CLI=$(/bin/hostname -I)

#TODO: need to remove hardcoding of ip here
if [ ! "$(echo $GET_IP_CLI|grep '^15.*')" = "" ] || \
   ([ ! "$(echo $GET_IP_CLI|grep '^10.*')" = "" ] && [ ! -z "$TUN0_IP" ]) || \
   [ ! "$(echo $GET_IP_CLI|grep '^16.*')" = "" ] ; then
    export PROXY='http://web-proxy.rose.hp.com:8080'
else
    unset PROXY
fi

if [ ! -z "$PROXY" ] &&  [ ! "$PROXY" = '"nil"' ]; then
echo "setting up proxy"
    export http_proxy=$PROXY
    export https_proxy=$http_proxy
    export HTTP_PROXY=$http_proxy
    export HTTPS_PROXY=$http_proxy
    export ftp_proxy=$(echo $http_proxy | sed 's/^http/ftp/g')
    export socks_proxy=$(echo $http_proxy | sed 's/^http/socks/g')
    export no_proxy=localhost,127.0.0.1,10.0.0.0/16,169.254.169.254
    if [ "$(id -u)" = "0" ] ; then # should only be done by root
    # clear out previous setting
    [ -f /etc/apt/apt.conf ] && cat /etc/apt/apt.conf | grep -v '::proxy' > /etc/apt/apt.conf
    # reset the apt.conf
      cat >> /etc/apt/apt.conf <<APT_CONF
Acquire::http::proxy "${http_proxy}";
Acquire::https::proxy "${http_proxy}";
Acquire::ftp::proxy "${ftp_proxy}";
Acquire::socks::proxy "${socks_proxy}";
APT_CONF
fi

else
    unset PROXY
    unset http_proxy
    unset https_proxy
    unset HTTP_PROXY
    unset HTTPS_PROXY
    unset ftp_proxy
    unset socks_proxy
    unset no_proxy
    [ "$(id -u)" = "0" ] && [ -f /etc/apt/apt.conf ] && cat /etc/apt/apt.conf | grep -v '::proxy' > /etc/apt/apt.conf
    echo "skiping proxy settings"
fi
