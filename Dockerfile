#
# Easy dockerfile to test my stuff
FROM ubuntu:bionic
LABEL Description="Wenlock dotfiles" Vendor="Wenlock Wizzard in a Blizzard LTD." Version="0.0.1" Maintainer="wenlock@github.com"

# Lets setup Docker in Docker using https://github.com/microsoft/vscode-dev-containers/tree/master/script-library

# See https://aka.ms/vscode-remote/containers/non-root-user for details.
ARG USERNAME=wenlock
ARG USER_UID=1000
ARG USER_GID=$USER_UID
# Common debian config
ARG UPGRADE_PACKAGES="true"
ARG INSTALL_ZSH="true"
ARG COMMON_SCRIPT_SOURCE="https://raw.githubusercontent.com/microsoft/vscode-dev-containers/master/script-library/common-debian.sh"
ARG COMMON_SCRIPT_SHA="dev-mode"
# Docker script args, location, and expected SHA - SHA generated on release
ARG DOCKER_SCRIPT_SOURCE="https://raw.githubusercontent.com/microsoft/vscode-dev-containers/master/script-library/docker-debian.sh"
ARG DOCKER_SCRIPT_SHA="dev-mode"
ARG ENABLE_NONROOT_DOCKER="true"
ARG SOURCE_SOCKET=/var/run/docker-host.sock
ARG TARGET_SOCKET=/var/run/docker.sock
RUN export DEBIAN_FRONTEND=noninteractive \
    && apt-get update \
    && apt-get -y install --no-install-recommends \
       apt-transport-https \    
       apt-utils \
       dialog \
       ca-certificates \
       coreutils \
       curl \
       git \
       gnupg2 \
       gnupg-agent \
       gosu \
       less \
       lsb-release \
       openssh-client \
       procps \
       socat \
       software-properties-common \
    2>&1 \
    #
    # common debian config like sudo, add user, etc
    && curl -sSL  ${COMMON_SCRIPT_SOURCE} -o /tmp/common-setup.sh \
    && ([ "${COMMON_SCRIPT_SHA}" = "dev-mode" ] || (echo "${COMMON_SCRIPT_SHA} */tmp/common-setup.sh" | sha256sum -c -)) \
    && /bin/bash /tmp/common-setup.sh "${INSTALL_ZSH}" "${USERNAME}" "${USER_UID}" "${USER_GID}" "${UPGRADE_PACKAGES}" \
    && rm /tmp/common-setup.sh \
    #
    # Install dockerd
    && curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add - \
    && add-apt-repository \
        "deb [arch=amd64] https://download.docker.com/linux/ubuntu \
        $(lsb_release -cs) \
        stable" \
    && apt-get update \
    && apt-get -y install --no-install-recommends \
        docker-ce \
        docker-ce-cli \
        containerd.io \
    #
    # Use Docker script from script library to set things up (installs: docker, docker-compose, sets up dind, and a bunch of other stuff)
    && curl -sSL $DOCKER_SCRIPT_SOURCE -o /tmp/docker-setup.sh \
    && ([ "${DOCKER_SCRIPT_SHA}" = "dev-mode" ] || (echo "${DOCKER_SCRIPT_SHA} */tmp/docker-setup.sh" | sha256sum -c -)) \
    && /bin/bash /tmp/docker-setup.sh "${ENABLE_NONROOT_DOCKER}" "${SOURCE_SOCKET}" "${TARGET_SOCKET}" "${USERNAME}" \
    && rm /tmp/docker-setup.sh \
    #
    # Clean up
    && apt-get autoremove -y \
    && apt-get clean -y \
    && rm -rf /var/lib/apt/lists/*
# try running as root
RUN docker-compose --version \
    && docker --version

VOLUME /var/lib/docker

ENV DOCKER_CHANNEL=stable
ENV DOCKER_EXTRA_OPTS="--default-address-pool base=10.88.0.0/22,size=28 --storage-driver overlay2 --log-level error"
ENV DIND_COMMIT=52379fa76dee07ca038624d639d9e14f4fb719ff

COPY opt/bin/dockerd-entrypoint.sh /usr/local/bin/dockerd-entrypoint.sh
RUN curl -fL -o /usr/local/bin/dind "https://raw.githubusercontent.com/moby/moby/${DIND_COMMIT}/hack/dind" \
    && chmod +x /usr/local/bin/dind \
    && chmod +x /usr/local/bin/dockerd-entrypoint.sh \
    && usermod -a -G docker $USERNAME

WORKDIR /home/$USERNAME
USER $USERNAME

ENTRYPOINT ["/usr/local/bin/dockerd-entrypoint.sh"]
CMD ["sleep", "infinity"]