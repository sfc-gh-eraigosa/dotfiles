# syntax=docker/dockerfile:1
#
# CI test image: the repo installed on top of the prebuilt base (ubuntu + apt
# tooling + docker-ce/dind — see Dockerfile.base, published to GHCR by
# .github/workflows/docker-base.yml). Only these per-commit layers rebuild in
# CI; pull or build the base first (`make build-base` on a GHCR miss).
#
# Two-phase install for layer caching (install.sh --phase, default `all` on
# real machines is unchanged):
#   1. deps  — repo-INDEPENDENT runtime toolchains, external tools, OS packages.
#      Only the inputs that affect dependency installation are copied first, so
#      this expensive layer is REUSED across commits that don't touch opt/ or
#      sdk/ or install.sh (the common case). A commit that DOES change a deps
#      installer correctly busts this layer and re-validates it — so unlike
#      baking deps into the base image, per-PR coverage of deps changes is kept.
#   2. config — repo-DEPENDENT profile links, AI-skill sync, and sdk binary
#      builds. Runs after the full-repo COPY, so it re-runs every commit (fast).
ARG BASE_IMAGE=ghcr.io/sfc-gh-eraigosa/dotfiles-base:latest
FROM ${BASE_IMAGE}
ARG USERNAME=agent

WORKDIR /home/$USERNAME

# --- Phase 1: deps (cache-friendly; inputs are stable) --------------------
COPY --chown=$USERNAME:$USERNAME opt git/dotfiles/opt
COPY --chown=$USERNAME:$USERNAME sdk git/dotfiles/sdk
COPY --chown=$USERNAME:$USERNAME install.sh Makefile git/dotfiles/

# goenv derives GOPATH as $HOME/go/<version>, so $HOME/go must be writable by the
# build user. BuildKit creates any MISSING PARENT of a mount target as root, and
# the cache mount below targets $HOME/go/pkg/mod — so without this, `agent` cannot
# mkdir $HOME/go/<version> and every `go build` dies with
#   go: could not create module cache: mkdir /home/agent/go/<ver>: permission denied
# Most sdk build.sh scripts treat that as non-fatal (warn and skip), so it stayed
# invisible until the sdk builds actually began running in the config layer.
RUN mkdir -p /home/$USERNAME/go && chown $USERNAME:$USERNAME /home/$USERNAME/go

# Pin the Go caches to the exact mount targets. Left to itself, goenv's VERSIONED
# GOPATH puts GOMODCACHE at $HOME/go/<version>/pkg/mod, which never matches the
# mount at $HOME/go/pkg/mod — so the module-cache mount was caching nothing.
ENV GOMODCACHE=/home/$USERNAME/go/pkg/mod
ENV GOCACHE=/home/$USERNAME/.cache/go-build

USER $USERNAME
RUN --mount=type=cache,target=/home/${USERNAME}/.cache,uid=1000,gid=1000 \
    --mount=type=cache,target=/home/${USERNAME}/go/pkg/mod,uid=1000,gid=1000 \
    /home/$USERNAME/git/dotfiles/install.sh --phase deps

# --- Phase 2: config (full repo; re-runs per commit) ----------------------
COPY --chown=$USERNAME:$USERNAME . git/dotfiles/
RUN --mount=type=cache,target=/home/${USERNAME}/.cache,uid=1000,gid=1000 \
    --mount=type=cache,target=/home/${USERNAME}/go/pkg/mod,uid=1000,gid=1000 \
    /home/$USERNAME/git/dotfiles/install.sh --phase config
RUN /home/$USERNAME/git/dotfiles/ai/antigravity/scripts/sanity_check.sh
# Explicit Claude sanity at build-time — fails the image if the npm install or hook setup broke.
# Sources .profile so the nvm-managed `claude` binary is on PATH.
RUN bash -c "source ~/.profile && /home/$USERNAME/git/dotfiles/ai/claude/scripts/sanity_check.sh"

# ENTRYPOINT and CMD (dockerd-entrypoint.sh / sleep infinity) are inherited
# from the base image.
