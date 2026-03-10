#!/bin/sh

# TODO: need a way to check if cortext cli is installed and if not prompt and ask to install it
if ! cortex --version; then
    curl -LsS https://ai.snowflake.com/static/cc-scripts/install.sh | sh
fi


# TODO: need a way to check if this is configured and if not configure it
podman machine set --rootful

# TODO: need a way to check if podman is started, and if not start it
#  podman machine stop; podman machine start
podman machine start



# podman listens on: /var/folders/xr/0zsny9_j6h58dcd3f2rztsvm0000gn/T/podman/podman-machine-default-api.sock

# How to install as a service
#   sudo /opt/homebrew/Cellar/podman/5.7.1/bin/podman-mac-helper install
#   podman machine stop; podman machine start

# setup docker host to connect to with docker
export DOCKER_HOST='unix:///var/folders/xr/0zsny9_j6h58dcd3f2rztsvm0000gn/T/podman/podman-machine-default-api.sock'
echo "unset DOCKER_HOST, if you need to disable podman in shell"


if [[ ! -f ~/.snowflake/config.toml ]]; then
    echo "setup ~/.snowflake/config.toml"
    echo '[connections.Snowhouse]
    account = "snowhouse"
    user = "<your_username>"
    authenticator = "externalbrowser"'
fi
export PATH="$HOME/.snowflake/bin:$PATH"
