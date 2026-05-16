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
if [ ! $? -eq 0 ] ; then
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
    _start=""
    _ruby_image="ruby"
    _options='--network rails -e http_proxy -e https_proxy -e no_proxy -e RAILS_ENV -e BUNDLE_GEMFILE'
    if [ "\$2" = "--version" ]; then
      echo "Running in docker"
    fi
    if [ "\$1" = "rails-server" ]; then
      _ruby_image="ruby:rails"
      _options="\${_options} --name rails-server --link xamp -p 3000:3000"
      _start="rails server -b 0.0.0.0"
      shift
    elif [ "\$1" = "rails-xamp" ]; then
      docker inspect xamp > /dev/null 2<&1 && \
          echo 'recreating xamp container ...' && \
          docker rm -f xamp
      echo 'Starting xamp server'
      _ruby_image="tomsik68/xampp"
      _options="\${_options} --name xamp -d -p 8080:80"
      shift
    elif [ "\$1" = "rails" ] && [ "\$2" = "console" ]; then
      _ruby_image="ruby:rails"
      _options="\${_options} -it --rm --link rails-server --link xamp"
      _start="bash"
      shift; shift;
    elif [ "\$1" = "rails" ]; then
      _options="\${_options} --rm --link xamp --link rails-server"
      _ruby_image="ruby:rails"
    fi
    eval "docker run -it --rm \
              \${_options}  \
               -v ruby-bundle:/usr/local/bundle \
               -v \$(pwd):/workspace \
               --workdir /workspace \${_ruby_image} \$_start \$@"
}
function f-ruby-install-rails {
    eval 'docker run -it --name ruby-rails \
                     -v ruby-bundle:/usr/local/bundle \
                     -e http_proxy \
                     -e https_proxy \
                     -e no_proxy \
                     -e HTTP_PROXY \
                     -e HTTPS_PROXY \
                     -e NO_PROXY \
    ruby bash -c "apt-get update && \
    apt-get install -y --no-install-recommends \
        nodejs \
        mysql-client \
        postgresql-client \
        sqlite3 && \
    rm -rf /var/lib/apt/lists/* && \
    gem install rails --version 5.0.1"'
    docker commit ruby-rails ruby:rails
    docker rm -f ruby-rails
}

function f-ruby-docker-disable {
  for cmd in ruby rake bundle gem rails rails-xamp rails-server rubocop rspec; do
      eval 'unalias \$cmd'
  done
}

alias ruby-docker-setup='f-ruby-docker-setup'
alias ruby-install-rails='f-ruby-install-rails'
alias ruby-docker-disable='f-ruby-docker-disable'
for cmd in ruby rake bundle gem rails rails-xamp rails-server rubocop rspec; do
    eval 'alias \$cmd="f-ruby-docker-run \$cmd"'
done

EOF

chmod 755 $ALIAS_ENV_SCRIPT
# source $ALIAS_ENV_SCRIPT

echo "RUN command : source $ALIAS_ENV_SCRIPT; ruby-docker-setup"
echo '  to setup alias requirements'
