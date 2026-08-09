#
# CI test image: the repo installed on top of the prebuilt base (ubuntu + apt
# tooling + docker-ce/dind — see Dockerfile.base, published to GHCR by
# .github/workflows/docker-base.yml). Only these per-commit layers rebuild in
# CI; pull or build the base first (`make build-base` on a GHCR miss).
ARG BASE_IMAGE=ghcr.io/sfc-gh-eraigosa/dotfiles-base:latest
FROM ${BASE_IMAGE}
ARG USERNAME=agent

WORKDIR /home/$USERNAME
COPY --chown=$USERNAME:$USERNAME . git/dotfiles/
USER $USERNAME
RUN /home/$USERNAME/git/dotfiles/install.sh
RUN /home/$USERNAME/git/dotfiles/ai/antigravity/scripts/sanity_check.sh
# Explicit Claude sanity at build-time — fails the image if the npm install or hook setup broke.
# Sources .profile so the nvm-managed `claude` binary is on PATH.
RUN bash -c "source ~/.profile && /home/$USERNAME/git/dotfiles/ai/claude/scripts/sanity_check.sh"

# ENTRYPOINT and CMD (dockerd-entrypoint.sh / sleep infinity) are inherited
# from the base image.
