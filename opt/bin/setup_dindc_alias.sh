#!/bin/bash
#vim: setlocal ft=sh:
# (c) Copyright 2014 Hewlett-Packard Development Company, L.P.
#
#    Licensed under the Apache License, Version 2.0 (the "License");
#    you may not use this file except in compliance with the License.
#    You may obtain a copy of the License at
#
#        http://www.apache.org/licenses/LICENSE-2.0
#
#    Unless required by applicable law or agreed to in writing, software
#    distributed under the License is distributed on an "AS IS" BASIS,
#    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#    See the License for the specific language governing permissions and
#    limitations under the License.


# Copyright 2012 Hewlett-Packard Development Company, L.P
#
# dindc aliases help manage docker aliases for working with docker tools such
# as ucp, dtr, compose, machine and docker engine tools.
#

SCRIPT_NAME=.dindcenv
ALIAS_ENV_SCRIPT="$HOME/$SCRIPT_NAME"

# VERIFY THAT docker is installed
docker --version 2>&1 >/dev/null
DOCKER_IS_AVAILABLE=$?
if [ $DOCKER_IS_AVAILABLE -eq 0 ]; then
    docker --version
else
    echo 'ERROR: DOCKER is not installed, try running : curl https://get.docker.com | sh'
    read -p 'Press any key to exit...'
    exit 1
fi


grep -e "${SCRIPT_NAME}" ~/.bash_aliases > /dev/null 2<&1
if [[ ! $? -eq 0 ]] ; then
    cat >> ~/.bash_aliases << DINDC_ALIASES

#
# Source environment shortcuts
#
if [ -f $ALIAS_ENV_SCRIPT ] ; then
    source $ALIAS_ENV_SCRIPT
else
    echo "$SCRIPT_NAME is missing, you can install with : . opt/bin/setup_dindc_alias.sh"
fi
DINDC_ALIASES
fi

grep -e "${SCRIPT_NAME}" ~/.profile > /dev/null 2<&1
if [[ ! $? -eq 0 ]] ; then
    echo "# Source git environment shortcuts" >> ~/.profile
    echo ". $ALIAS_ENV_SCRIPT" >> ~/.profile
fi

echo "#!/bin/bash" > $ALIAS_ENV_SCRIPT
echo "# docker environment shortcuts" >> $ALIAS_ENV_SCRIPT
cat >> $ALIAS_ENV_SCRIPT << EOF
BUNDLE_ROOT_DIR="\$HOME/.config/dindcenv"
alias dindc-logout='fdindc-logout \$@'
alias dindc-login='fdindc-login \$@'
alias dindc-help='fdindc-help'

function fdindc-commands {
    _commands=\$(alias|grep 'dindc-'|awk -F= '{print \$1}'|sed 's/alias //g' | awk '{printf \$1", "}' | sed 's/, \$//g')
    echo \$_commands
}

function fdindc-help {
    _commands=\$(fdindc-commands)

    echo "commands available : \$_commands"
    cat << HELP_TXT

  Summary:

    dindc-alias's are a set of helper tools to make it easier to interact with docker.
    Tired of trying to remember long worthless command lines?
    Make it simple with dindc alias...

    Commands will start with dindc-*, use dindc-help for instrucitons on how
    to use it.

  Available Commands:

  \$_commands

  Detailed Help:
  {} - are optional
  <> - are required


  dindc-help
    Get help for dindc aliases.

  dindc-login {ucp url} {ucp username}
    Use this alias to login to a particular ucp server from the command shell.
    dindc-login will store some defaults from previous login attempts in
    $HOME/.dindc.defaults, you can edit this key/value file to change the
    defaults.  Once you login, dindc-login will show docker info for your
    client.

  dindc-logout
    reset docker settings so that they are pointing to local docker client again
    and remove settings for login.

HELP_TXT

}
#
# common functions
#
function get-tempfile {
    _temp_prefix=temp
    [ ! -z "\${1}" ] && _temp_prefix=\$1
    [ ! -z "\${2}" ] && _temp_file=\$2
    if [ -z "\${_temp_file}" ] || [ ! -f "\${_temp_file}" ] ; then
        export _temp_file=\$( mktemp -t \${_temp_prefix}.XXXXXXXXXX )
    fi
    echo -n \$_temp_file
}
function clean-tempfile {
    _prefix=\$1
    _skip_file=\$2
    [ ! -z "\${_skip_file}" ] && find /tmp/\$_prefix* -type f -not -name "\$(basename \$_skip_file)" | xargs rm -f {} >/dev/null 2<&1
}

#Example: readparam "GIT_USER"
function readparam {
    FILE=~/git.properties
    R=""
    if [ -f "\$FILE" ]  ; then
        VALUE=\$(grep -E "^ *\$1" \$FILE  | cut -f2 -d'=')
        [ -n "\$VALUE" ] && R="\$VALUE"
    fi
    echo "\$R";
}

#Example: writeparam "GIT_USER" "chavezom"
function writeparam {
    FILE=~/git.properties

    if [ ! -f \$FILE ]  ; then
        touch \$FILE
    fi

    grep -E "^ *\$1=" \$FILE 2>&1 >/dev/null
    PARAM_EXISTS=\$?

    if [ \$PARAM_EXISTS -eq 0 ]; then
        cat \$FILE |grep -v -e "^\$1" > "\$FILE.bak"
        mv "\$FILE.bak" "\$FILE"
        echo "\$1=\$2" >> \$FILE
        else
        echo "\$1=\$2" >> \$FILE
    fi
}

function isWindows {
    [ "\$OS" == "Windows_NT" ] && return 1
    return 0
}

# do logout
function fdindc-logout {
    if [ ! -z "\$DINDC_LAST_BUNDLE_DIR" ] ; then
       echo 'unset DOCKER_API_VERSION && unset DOCKER_HOST && unset DOCKER_TLS_VERIFY && unset DOCKER_CERT_PATH' >> "\$DINDC_LAST_BUNDLE_DIR/unset.env"
       source "\$DINDC_LAST_BUNDLE_DIR/unset.env"
       writeparam "DINDC_LAST_BUNDLE_DIR" ""
       rm -rf "\$DINDC_LAST_BUNDLE_DIR"
       echo 'Logout completed'
    else
        echo 'No logout required, you can use dindc-login to login'
    fi
}

# set api version for client to match server
function dindc-setversion {
  local _apiversion=\$(docker version --format '{{.Server.APIVersion}}')
  if [ ! -z "\$_apiversion" ]; then
    echo "(dindc-login) - setting api version to \$_apiversion"
    export DOCKER_API_VERSION=\$_apiversion
  fi
}

# do ucp login
# <ucp url> <ucp login> <ucp password>
function fdindc-login {

    # determine the ucp url to use
    current_url=\$1
    if [ -z "\$current_url" ]; then
        [ -z "\$current_url" ] && current_url=\$DINDC_DEFAULT_UCPURL
        echo -n "Enter the ucp url [\$current_url]: "
        read ucp_url
    else
        ucp_url=\$current_url
    fi

    if [ -z "\$ucp_url" ]; then
        export DINDC_DEFAULT_UCPURL=\$current_url
        writeparam "DINDC_DEFAULT_UCPURL" "\$current_url"
    else
        export DINDC_DEFAULT_UCPURL=\$ucp_url
        writeparam "DINDC_DEFAULT_UCPURL" "\$ucp_url"
    fi

    if [ "\$(echo \$DINDC_DEFAULT_UCPURL | rev | cut -c 1-1)" = "/" ]; then
        export DINDC_DEFAULT_UCPURL=\${DINDC_DEFAULT_UCPURL: : -1}
    fi


    # determine the ucp login
    current_user=\$2
    if [ -z "\$current_user" ]; then
        [ -z "\$current_user" ] && current_user=\$DINDC_DEFAULT_UCPUSER
        echo -n "Enter the ucp username [\$current_user]: "
        read ucp_user
    else
        ucp_user=\$current_user
    fi

    if [ -z "\$ucp_user" ]; then
        export DINDC_DEFAULT_UCPUSER=\$current_user
        writeparam "DINDC_DEFAULT_UCPUSER" "\$current_user"
    else
        export DINDC_DEFAULT_UCPUSER=\$ucp_user
        writeparam "DINDC_DEFAULT_UCPUSER" "\$ucp_user"
    fi


    # determine the ucp password
    current_password=\$3
    if [ -z "\$current_password" ]; then
        echo -n "Enter the ucp password : "
        read -sr ucp_password
    else
        ucp_password=\$current_password
    fi
    echo ""

    # clean up last creds because we're about to login ,DINDC_LAST_BUNDLE_DIR
    if [ ! -z "\$DINDC_LAST_BUNDLE_DIR" ] ; then
        rm -rf "\$DINDC_LAST_BUNDLE_DIR"
    fi

    # get a temp path and clean up the temp file generated
    _temp=\$(get-tempfile)
    rm -f \$_temp

    # setup the bundle dir location
    export DINDC_LAST_BUNDLE_DIR="\${BUNDLE_ROOT_DIR}\$(get-tempfile)"
    writeparam "DINDC_LAST_BUNDLE_DIR" "\$DINDC_LAST_BUNDLE_DIR"
    mkdir -p "\${DINDC_LAST_BUNDLE_DIR}"

    echo "Please wait ... downloading creds from \$DINDC_DEFAULT_UCPURL"

    jsonauth=\$(curl -sk -d "{\\"username\\":\\"\${DINDC_DEFAULT_UCPUSER}\\",\\"password\\":\\"\$ucp_password\\"}" \
        --url "\$DINDC_DEFAULT_UCPURL/auth/login" )
    if [ "\$jsonauth" = "unauthorized" ] ; then
        echo "ERROR:  getting an auth token from \$DINDC_DEFAULT_UCPURL failed: unauthorized"
        return 1
    fi
    authtoken=\$(echo \$jsonauth | jq -r '.auth_token')
    if [ -z "\$authtoken" ]; then
        echo "ERROR: unable to get a valid auth token, \$jsonauth"
        return 1
    fi
    curl -sk -H "Authorization: Bearer \$authtoken" \
         --url "\$DINDC_DEFAULT_UCPURL/api/clientbundle" \
         -o "\${DINDC_LAST_BUNDLE_DIR}/bundle.zip" && \
    unzip -o -q "\${DINDC_LAST_BUNDLE_DIR}/bundle.zip" -d "\${DINDC_LAST_BUNDLE_DIR}"

    _cwd=\$(pwd)
    cd "\${DINDC_LAST_BUNDLE_DIR}"
    source ./env.sh
    dindc-setversion
    cd \$_cwd
    echo "login complete! use docker info to check your connection and dindc-logout to reset/remove your creds"
#    docker info
}

[ "\$(readparam 'DINDC_LAST_BUNDLE_DIR')" != "" ] && export DINDC_LAST_BUNDLE_DIR=\$(readparam "DINDC_LAST_BUNDLE_DIR")
[ "\$(readparam 'DINDC_DEFAULT_UCPURL')" != "" ] && export DINDC_DEFAULT_UCPURL=\$(readparam "DINDC_DEFAULT_UCPURL")
[ "\$(readparam 'DINDC_DEFAULT_UCPUSER')" != "" ] && export GIT_URL=\$(readparam "DINDC_DEFAULT_UCPUSER")

# disable till we find a workaround to bug in vagrant
if [[ ! "\$(whoami)" = "vagrant" ]] ; then
    echo "commands available : \$(fdindc-commands)"
fi

EOF
chmod 755 $ALIAS_ENV_SCRIPT
. $ALIAS_ENV_SCRIPT
