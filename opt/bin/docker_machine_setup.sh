#!/bin/bash
export DOCKER_MACHINE_HOME=$HOME/opt/bin
export DOCKER_MACHINE_VERSION='latest'

function get_detected_os {
    uname_bin=$(which uname)
    if [ -f "${uname_bin}" ]; then
        # uname is Darwin or Linux?
        os_type="$(uname -s)"
        if [ "${os_type}" = "Linux" ]; then
            # this is a linux distro
            if [ "$(uname -i)" = "x86_64" ]; then
                detected_os="linux-amd64"
            else
                detected_os="linux-386"
            fi
        elif [ "${os_type}" = "Darwin" ]; then
            # this is a mac distro
            if [ "$(uname -p)" = "i386" ]; then
                detected_os="darwin-amd64"
            else
                detected_os="darwin-386"
            fi
        else
            # not mac or linux, check for windows
            # PROGRAMW6432
            if [ ! -z "$PROGRAM6432" ]; then
                detected_os="windows-amd64.exe"
            else
                detected_os="windows-386.exe"
            fi
        fi
    else
        # check for ProgramFiles var in case it's windows
        # PROGRAMW6432
        if [ ! -z "$PROGRAM6432" ]; then
            detected_os="windows-amd64.exe"
        else
            detected_os="windows-386.exe"
        fi
    fi
}

if [ "$1" = "-f" ] ; then
    rm -f $DOCKER_MACHINE_HOME/docker-machine
fi
# setup $DOCKER_MACHINE_HOME/docker-machine
if [ "$DOCKER_MACHINE_VERSION" = "latest" ] ; then
    if [ -z "$(echo $(python --version 2>&1|grep Python))" ] ; then
        echo "ERROR: configure DOCKER_MACHINE_VERSION or install python to download"
        exit 1
    fi
    export DOCKER_MACHINE_VERSION=$(curl -s  https://api.github.com/repos/docker/machine/releases/latest |  python -c 'import sys, json; print json.load(sys.stdin)["name"]')
fi
if [ ! -f $DOCKER_MACHINE_HOME/docker-machine ] ; then
    get_detected_os
    download_url="https://github.com/docker/machine/releases/download/${DOCKER_MACHINE_VERSION}/docker-machine_${detected_os}"
    wget $download_url -O $DOCKER_MACHINE_HOME/docker-machine
    chmod +x $DOCKER_MACHINE_HOME/docker-machine
else
    echo "$DOCKER_MACHINE_HOME/docker-machine already exist.  use -f to force"
fi

exit 0
