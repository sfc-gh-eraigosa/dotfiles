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

SCRIPT_NAME=.ruby.env
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

grep -e "${SCRIPT_NAME}" "$HOME/.profile" > /dev/null 2<&1
if [[ ! $? -eq 0 ]] ; then
    cat >> "$HOME/.profile" << RUBY_DOCKER

#
# ruby docker environment aliases
#
if [ -f $ALIAS_ENV_SCRIPT ] ; then
    source $ALIAS_ENV_SCRIPT
else
    echo "$SCRIPT_NAME is missing, you can install with : . opt/bin/setup_ruby-docker.sh"
fi
RUBY_DOCKER
fi

echo "#!/bin/bash" > $ALIAS_ENV_SCRIPT
echo "# ruby docker environment shortcuts" >> $ALIAS_ENV_SCRIPT
cat >> $ALIAS_ENV_SCRIPT << EOF

function f-ruby-docker-setup {
    docker volume create ruby-bundle
}
function f-ruby-docker-run {
    if [ "\$2" = "--version" ]; then
      echo "Running in docker"
    fi
    docker run -it --rm -v ruby-bundle:/usr/local/bundle -v \$(pwd):/workspace --workdir /workspace ruby \$@
}
function f-ruby-docker-install {
    gem install rubocop
    gem install rspec
    gem install rails
}

function f-ruby-docker-disable {
  for cmd in ruby rake bundle gem rails rubocop rspec; do
      eval 'unalias \$cmd'
  done
}

alias ruby-docker-setup='f-ruby-docker-setup'
alias ruby-docker-install='f-ruby-docker-install'
alias ruby-docker-disable='f-ruby-docker-disable'
for cmd in ruby rake bundle gem rails rubocop rspec; do
    eval 'alias \$cmd="f-ruby-docker-run \$cmd"'
done

EOF

chmod 755 $ALIAS_ENV_SCRIPT
# source $ALIAS_ENV_SCRIPT

echo "RUN command : source $ALIAS_ENV_SCRIPT; ruby-docker-setup"
echo '  to setup alias requirements'
