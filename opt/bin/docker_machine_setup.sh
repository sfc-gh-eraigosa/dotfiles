#!/bin/bash
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

# setup opt/bin/docker-machine
if [ ! -f ~/opt/bin/docker-machine ] ; then
    get_detected_os
    download_url="https://github.com/docker/machine/releases/download/v0.2.0/docker-machine_${detected_os}"
    wget $download_url -O ~/opt/bin/docker-machine
    chmod +x ~/opt/bin/docker-machine
fi
