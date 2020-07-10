#!/bin/bash
set -e
## Launch Docker Daemon
echo "==> Launching the Docker daemon..."
nohup dind dockerd $DOCKER_EXTRA_OPTS &
counter=1
while(! docker info > /dev/null 2>&1); do
    if [ $counter -lt 30 ];then
        echo "==> Waiting for the Docker daemon to come online..."
        sleep 1
        counter+=1
    else
        echo "Failed to bring up Docker daemon after $counter tries"
        cat nohup.out
        exit 1
    fi
done
echo "==> Docker Daemon is up and running!"
exec "$@"