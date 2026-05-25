#!/bin/bash
#
function install_zsh_centos7() {
    sudo yum update -y
    sudo yum install -y git make ncurses-devel gcc autoconf man
    git clone -b zsh-5.7.1 https://github.com/zsh-users/zsh.git /tmp/zsh
    (
        cd /tmp/zsh
        ./Util/preconfig
        ./configure
        sudo make -j 20 install.bin install.modules install.fns
    )
}

export BASE_DIR="$(cd "$(dirname $0)" && pwd)"

# --- Authenticate sudo up front -------------------------------------------
# The rest of this installer runs several privileged steps (apt/yum installs,
# the GitHub CLI apt-repo setup, chsh). Caching sudo credentials once here means
# the long, otherwise-unattended run never stalls on a password prompt midway.
# A background keep-alive refreshes the timestamp until this script exits so it
# can't lapse mid-install. Skipped when already root or when sudo is absent
# (e.g. minimal containers); a failed auth is non-fatal — individual privileged
# steps still run and warn on their own.
if [ "$(id -u)" -ne 0 ] && command -v sudo &> /dev/null; then
  echo "Requesting sudo access up front (used for package installs, repo setup, chsh)..."
  if sudo -v; then
    # Refresh the sudo timestamp every 50s (well under the default 15m timeout)
    # until install.sh exits, so a slow build can't let credentials lapse.
    while true; do sudo -n true 2>/dev/null; sleep 50; kill -0 "$$" 2>/dev/null || exit; done &
    SUDO_KEEPALIVE_PID=$!
    trap 'kill "$SUDO_KEEPALIVE_PID" 2>/dev/null' EXIT
  else
    echo "WARNING: could not cache sudo credentials; privileged steps may prompt or be skipped."
  fi
fi

git config --global pager.branch false
git config --global push.default current

[ ! -d "${HOME}/git" ] && mkdir -p "${HOME}/git"

# skip login check for sshd
touch "${HOME}/.gitenv.nologin"

# Ensure ~/opt is a symlink to the repo's opt directory
if [ -L "${HOME}/opt" ] && [ "$(readlink "${HOME}/opt")" = "${BASE_DIR}/opt" ]; then
  : # Already correctly linked
elif [ -e "${HOME}/opt" ]; then
  echo "WARNING: ${HOME}/opt exists but is not linked to dotfiles. Backing up to ${HOME}/opt.bak"
  mv "${HOME}/opt" "${HOME}/opt.bak"
  ln -sf "${BASE_DIR}/opt" "${HOME}/opt"
else
  ln -sf "${BASE_DIR}/opt" "${HOME}/opt"
fi

# Windows/WSL only: deploy opt/Desktop/* onto the real Windows Desktop.
# Logic is isolated in opt/bin/install_windows.sh for clarity.
if [ -f "${BASE_DIR}/opt/bin/install_windows.sh" ]; then
  bash "${BASE_DIR}/opt/bin/install_windows.sh" "${BASE_DIR}"
fi

# Source hardware detection
if [ -f "${BASE_DIR}/opt/lib/hardware.sh" ]; then
  . "${BASE_DIR}/opt/lib/hardware.sh"
  if is_jetson; then
    echo "Detected NVIDIA Jetson hardware. Applying specific optimizations..."
    # On Jetson, ensure we have necessary basic tools for dev
    [ -z "$(command -v tegrastats)" ] && echo "WARNING: tegrastats not found. You may need to install JetPack."
    
    # Setup jtop and stats
    if [ -f "${BASE_DIR}/opt/scripts/system/setup_jtop.sh" ]; then
      echo "Setting up jtop and jetson-stats..."
      "${BASE_DIR}/opt/scripts/system/setup_jtop.sh"
    fi
    
    # Set Chromium as default browse
    if command -v apt-get &> /dev/null; then
      echo "Ensuring Chromium is installed and set as default..."
      sudo apt-get install -y -qq chromium-browse
      
      # Set as default in update-alternatives
      sudo update-alternatives --set x-www-browser /usr/bin/chromium-browser 2>/dev/null || true
      sudo update-alternatives --set gnome-www-browser /usr/bin/chromium-browser 2>/dev/null || true
      
      # Set for xdg-utils if in a desktop environment
      if command -v xdg-settings &> /dev/null; then
        xdg-settings set default-web-browser chromium-browser.desktop 2>/dev/null || true
      fi
    fi
  fi
fi

for file in $(find "${BASE_DIR}/opt/profiles" -type f); do
    filename=$(basename "$file")
    # Skip metadata and non-profile files
    [[ "$filename" == "Brewfile" ]] && continue
    [[ "$filename" == "packages.tsv" ]] && continue
    [[ "$filename" == "requirements.txt" ]] && continue
    [[ "$filename" == "GEMINI.md" ]] && continue
    [[ "$filename" == "CLAUDE.md" ]] && continue
    
    echo "Creating symlink to $file in home directory."
    ln -sf "${file}" "${HOME}/${filename}"
done

# force a few
for file in ".profile" ".zshrc" ".bash_logout" ".bashrc"; do
  ln -sf "${BASE_DIR}/opt/profiles/${file}" "${HOME}/${file}"
done 

# Shared skill sync — links every SKILL.md into BOTH ~/.agents/skills (Gemini)
# and ~/.claude/skills (Claude). Single source of truth for both assistants.
if [ -f "${BASE_DIR}/opt/scripts/system/sync-skills.sh" ]; then
    bash "${BASE_DIR}/opt/scripts/system/sync-skills.sh"
fi

# Gemini CLI Configuration (Policies, Commands, Aliases)
if [ -f "${BASE_DIR}/opt/scripts/system/install_gemini_skills.sh" ]; then
    # sync-skills handles the skill links now; this only does Gemini-specific config.
    "${BASE_DIR}/opt/scripts/system/install_gemini_skills.sh"
fi

# Claude Code Configuration (Settings, Commands, Hooks, Aliases)
if [ -f "${BASE_DIR}/opt/scripts/system/install_claude_skills.sh" ]; then
    # sync-skills handles the skill links now; this only does Claude-specific config.
    "${BASE_DIR}/opt/scripts/system/install_claude_skills.sh"
fi

NIX_MANAGED_FILE="${HOME}/.config/nix_managed"

if [ -f "$NIX_MANAGED_FILE" ]; then
  echo "Skipping system package install because the env is managed with nix, found $NIX_MANAGED_FILE"
else
  # Install the curated common-core packages. They are defined once in
  # opt/profiles/packages.tsv; the per-platform installers in opt/bin translate
  # that list to the right package manager (apt on Debian/Ubuntu/WSL, Homebrew
  # on macOS). These can also be run by hand: `pkg-install` (auto-detect) o
  # `pkg-install-apt` / `pkg-install-brew` directly.
  if [ -x "${BASE_DIR}/opt/bin/pkg-install" ]; then
    "${BASE_DIR}/opt/bin/pkg-install" || echo "WARNING: package install reported problems; continuing."
  fi

  # RHEL/CentOS is not manifest-driven yet; keep the legacy yum path working.
  if command -v yum &> /dev/null; then
    sudo yum install -y -q \
      htop \
      jq \
      lsof \
      net-tools \
      psmisc \
      zsh

    install_zsh_centos7
  fi

  # Set zsh as the default shell on every platform — but only when zsh is
  # actually installed, so we never blank the login shell by passing an empty
  # path to chsh.
  ZSH_PATH="$(command -v zsh || true)"
  if [ -n "$ZSH_PATH" ]; then
    if [ "$SHELL" != "$ZSH_PATH" ]; then
      echo "Changing default shell to zsh ($ZSH_PATH)..."
      # ${USER:-$(id -un)} so chsh still gets a real name in non-login/root shells.
      sudo chsh -s "$ZSH_PATH" "${USER:-$(id -un)}" || echo "WARNING: could not change default shell to zsh."
    fi
  else
    echo "WARNING: zsh is not installed; leaving the default shell unchanged."
  fi
fi

# brew script for macOS: install the macOS-only extras from the Brewfile.
# (The cross-platform common core is installed above via pkg-install.)
if [[ "$(uname -s)" == "Darwin" ]]; then
  if command -v brew &> /dev/null; then
    echo "Detected macOS. Running brew bundle for macOS extras..."
    brew bundle --no-upgrade --file="${BASE_DIR}/opt/profiles/Brewfile" || {
      echo "WARNING: Some brew formulas failed to install. Review the output above."
      echo "         This is non-fatal — the rest of the setup will continue."
    }
    # Setup Vault as a standalone binary to avoid Xcode versioning issues
    if [ -f "${BASE_DIR}/opt/scripts/network/vault-setup.sh" ]; then
      "${BASE_DIR}/opt/scripts/network/vault-setup.sh"
    fi
  else
    echo "WARNING: Homebrew not found. Please install it: https://brew.sh/"
  fi
fi

# Install sops (secrets management). macOS gets it from the Brewfile above;
# Linux/WSL has no usable apt package, so install_sops.sh fetches the official
# static release binary into ~/opt/bin. Safe to re-run on any platform.
if [ -f "${BASE_DIR}/opt/scripts/system/install_sops.sh" ]; then
    echo "Installing sops..."
    "${BASE_DIR}/opt/scripts/system/install_sops.sh" || echo "WARNING: sops install reported problems; continuing."
fi

# only setup these scripts when docker is installed and responsive
if command -v docker &> /dev/null; then
    # Setup docker permissions for the current use
    "${BASE_DIR}/opt/scripts/docker/setup_docker_perms.sh"
    
    # Check if Docker daemon is actually running (timeout after 5 seconds)
    # Use gtimeout on macOS (coreutils), timeout on Linux
    TIMEOUT_CMD="timeout"
    command -v timeout &>/dev/null || TIMEOUT_CMD="gtimeout"
    if ($TIMEOUT_CMD 5 docker info &>/dev/null 2>&1) 2>/dev/null; then
        source "${BASE_DIR}/opt/scripts/docker/setup_ruby-docker.sh"
        source "${BASE_DIR}/opt/scripts/docker/setup_dindc_alias.sh"
    else
        echo "NOTE: Docker is installed but the daemon is not running. Skipping Docker-dependent setup."
    fi
fi

# Setup git environment aliases
if [ -f "${BASE_DIR}/opt/scripts/git/setup_git_alias.sh" ]; then
    source "${BASE_DIR}/opt/scripts/git/setup_git_alias.sh"
fi

# install/update goenv
if [[ "$(uname -s)" == "Darwin" ]] && command -v brew &> /dev/null; then
  if ! command -v goenv &> /dev/null; then
    echo "Installing goenv..."
    brew install goenv
  fi
else
  # Linux / Jetson / others
  if [ ! -d "${HOME}/.goenv" ]; then
    echo "Installing goenv..."
    git clone https://github.com/syndbg/goenv.git "${HOME}/.goenv"
  else
    echo "Updating goenv..."
    (cd "${HOME}/.goenv" && git pull)
  fi
fi

# Initialize goenv to install/update Go
# Pin to the version in .go-version so all hosts build with the same toolchain.
# Falls back to "latest" only if .go-version is missing.
export PATH="${HOME}/.goenv/bin:${PATH}"
if command -v goenv &> /dev/null; then
  eval "$(goenv init -)"
  if [ -f "${BASE_DIR}/.go-version" ]; then
    PINNED_GO_VERSION=$(tr -d '[:space:]' < "${BASE_DIR}/.go-version")
    echo "Ensuring Go ${PINNED_GO_VERSION} is installed (pinned via .go-version)..."
    goenv install -s "${PINNED_GO_VERSION}"
    goenv global "${PINNED_GO_VERSION}"
  else
    echo "WARNING: ${BASE_DIR}/.go-version not found; installing latest goenv-known Go."
    goenv install -s latest
    LATEST_INSTALLED=$(goenv versions --bare | tail -1)
    if [ -n "$LATEST_INSTALLED" ]; then
      goenv global "$LATEST_INSTALLED"
    fi
  fi
fi

# install/update pyenv
if [[ "$(uname -s)" == "Darwin" ]] && command -v brew &> /dev/null; then
  if ! command -v pyenv &> /dev/null; then
    echo "Installing pyenv..."
    brew install pyenv
  fi
else
  # Linux / Jetson / others
  if [ ! -d "${HOME}/.pyenv" ]; then
    echo "Installing pyenv..."
    git clone https://github.com/pyenv/pyenv.git "${HOME}/.pyenv"
  else
    echo "Updating pyenv..."
    (cd "${HOME}/.pyenv" && git pull)
  fi
fi

# install/update rbenv
if [[ "$(uname -s)" == "Darwin" ]] && command -v brew &> /dev/null; then
  if ! command -v rbenv &> /dev/null; then
    echo "Installing rbenv..."
    brew install rbenv
  fi
else
  # Linux / Jetson / others
  if [ ! -d "${HOME}/.rbenv" ]; then
    echo "Installing rbenv..."
    git clone https://github.com/rbenv/rbenv.git "${HOME}/.rbenv"
    # Also need ruby-build plugin for rbenv
    mkdir -p "${HOME}/.rbenv/plugins"
    git clone https://github.com/rbenv/ruby-build.git "${HOME}/.rbenv/plugins/ruby-build"
  else
    echo "Updating rbenv..."
    (cd "${HOME}/.rbenv" && git pull)
    if [ -d "${HOME}/.rbenv/plugins/ruby-build" ]; then
      (cd "${HOME}/.rbenv/plugins/ruby-build" && git pull)
    fi
  fi
fi

# install nvm
if [ ! -d "${HOME}/.nvm" ]; then
  echo "Installing nvm..."
  curl -fsSL -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash > /dev/null 2>&1
fi

# install Gemini CLI
if [ -f "${BASE_DIR}/opt/scripts/system/gemini_install.sh" ]; then
    echo "Installing Gemini CLI..."
    "${BASE_DIR}/opt/scripts/system/gemini_install.sh"
fi

# Google CLI (Gemini & Workspace) Setup
if [ -f "${BASE_DIR}/opt/scripts/system/google-cli-setup.sh" ]; then
    # This configures gws for BOTH Gemini CLI and Claude Code via shared skills.
    echo "Setting up Google CLI (Gemini & Workspace)..."
    "${BASE_DIR}/opt/scripts/system/google-cli-setup.sh"
fi

# Gemini settings
if [ -f "${BASE_DIR}/ai/gemini/settings.json" ]; then
    echo "Configuring Gemini settings..."
    mkdir -p "${HOME}/.gemini"
    # If it's a real file (not a symlink), back it up
    if [ -f "${HOME}/.gemini/settings.json" ] && [ ! -L "${HOME}/.gemini/settings.json" ]; then
        echo "  Backing up existing settings.json to settings.json.bak"
        mv "${HOME}/.gemini/settings.json" "${HOME}/.gemini/settings.json.bak"
    fi
    ln -sf "${BASE_DIR}/ai/gemini/settings.json" "${HOME}/.gemini/settings.json"
fi

# install Claude Code CLI (macOS via brew cask, Linux/WSL via npm)
if [ -f "${BASE_DIR}/opt/scripts/system/claude_install.sh" ]; then
    echo "Installing Claude Code CLI..."
    "${BASE_DIR}/opt/scripts/system/claude_install.sh"
fi

# Claude settings (re-link in case install order matters)
# settings.json is gitignored per-host. Seed from .template on first run, then
# symlink ~/.claude/settings.json to the local copy.
CLAUDE_SETTINGS="${BASE_DIR}/ai/claude/settings.json"
CLAUDE_SETTINGS_TEMPLATE="${BASE_DIR}/ai/claude/settings.json.template"
if [ ! -f "${CLAUDE_SETTINGS}" ] && [ -f "${CLAUDE_SETTINGS_TEMPLATE}" ]; then
    echo "Seeding ai/claude/settings.json from template (first run)"
    cp "${CLAUDE_SETTINGS_TEMPLATE}" "${CLAUDE_SETTINGS}"
fi
if [ -f "${CLAUDE_SETTINGS}" ]; then
    echo "Configuring Claude Code settings..."
    mkdir -p "${HOME}/.claude"
    if [ -f "${HOME}/.claude/settings.json" ] && [ ! -L "${HOME}/.claude/settings.json" ]; then
        echo "  Backing up existing settings.json to settings.json.bak"
        mv "${HOME}/.claude/settings.json" "${HOME}/.claude/settings.json.bak"
    fi
    ln -sf "${CLAUDE_SETTINGS}" "${HOME}/.claude/settings.json"
fi
# build and install gss
if [ -f "${BASE_DIR}/src/gss/build.sh" ]; then
    echo "Installing gss (dotfiles manager)..."
    bash "${BASE_DIR}/src/gss/build.sh"
    if [ -f "${HOME}/opt/bin/gss" ]; then
        echo "--------------------------------------------------"
        "${HOME}/opt/bin/gss" version
        echo "--------------------------------------------------"
    fi
fi

# build and install tmux-mg
if [ -f "${BASE_DIR}/src/tmux-mgr/build.sh" ]; then
    echo "Installing tmux-mgr..."
    bash "${BASE_DIR}/src/tmux-mgr/build.sh"
    if [ -f "${HOME}/opt/bin/tmux-mgr" ]; then
        echo "--------------------------------------------------"
        "${HOME}/opt/bin/tmux-mgr" version
        echo "--------------------------------------------------"
        # install aliases
        "${HOME}/opt/bin/tmux-mgr" alias install
    fi
fi

# build and install wol
if [ -f "${BASE_DIR}/src/wol/build.sh" ]; then
    echo "Installing wol (Wake-on-LAN utility)..."
    bash "${BASE_DIR}/src/wol/build.sh"
    if [ -f "${HOME}/opt/bin/wol" ]; then
        echo "--------------------------------------------------"
        "${HOME}/opt/bin/wol" version
        echo "--------------------------------------------------"
    fi
fi


# install fnm
if ! command -v fnm &> /dev/null; then
  echo "Installing fnm..."
  curl -fsSL https://fnm.vercel.app/install | bash -s -- --skip-shell > /dev/null 2>&1
fi

# setup sshd server if requested
if [ -f "${HOME}/.sshd.env" ]; then
    "${BASE_DIR}/opt/scripts/network/sshd_run.sh"
fi

if [ -f "${HOME}/.gitrepos" ] ; then
  cd "${HOME}"
  "${HOME}/.gitrepos"
fi

# Load Nano Platform environment
for shell_config in "$HOME/.zshrc" "$HOME/.profile"; do
    if [ -f "$shell_config" ]; then
        if ! grep -q "\.nano_profile" "$shell_config"; then
            echo "Adding .nano_profile source to $shell_config"
            echo '[ -f "$HOME/.nano_profile" ] && . "$HOME/.nano_profile"' >> "$shell_config"
        fi
    fi
done
