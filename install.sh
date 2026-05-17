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

# Idempotent migration of opt/bin to opt/scripts
function migrate_opt_bin() {
  local repo_opt_bin="${BASE_DIR}/opt/bin"
  local repo_opt_scripts="${BASE_DIR}/opt/scripts"
  
  if [ ! -d "$repo_opt_scripts" ]; then
    echo "Migrating opt/bin to categorized opt/scripts..."
    mkdir -p "$repo_opt_scripts"/{git,docker,system,network,data,misc}
    
    # Git
    for f in git_add.sh git_branch.sh git_pull.sh git-local-master.sh git-reset.sh git-rm-mybranches.sh git-signoff.sh gh_issues.rb setup_git_alias.sh update_origin.sh; do
      [ -f "$repo_opt_bin/$f" ] && mv "$repo_opt_bin/$f" "$repo_opt_scripts/git/"
    done
    # Docker
    for f in docker_machine_setup.sh docker_up.sh dockerd-entrypoint.sh prepare_node_docker.sh setup_dindc_alias.sh setup_docker_perms.sh setup_ruby-docker.sh; do
      [ -f "$repo_opt_bin/$f" ] && mv "$repo_opt_bin/$f" "$repo_opt_scripts/docker/"
    done
    # System
    for f in coco_install.sh crouton-alias.sh enable-vmx.sh gemini_install.sh google-cli-setup.sh install_gemini_skills.sh nvm perf-toggle.sh setup_jtop.sh terminal-theme.sh; do
      [ -f "$repo_opt_bin/$f" ] && mv "$repo_opt_bin/$f" "$repo_opt_scripts/system/"
    done
    # Network
    for f in import-cert.sh proxy.sh remote-setup.sh ssh-find sshd_run.sh vault-login.sh vault-setup.sh; do
      [ -f "$repo_opt_bin/$f" ] && mv "$repo_opt_bin/$f" "$repo_opt_scripts/network/"
    done
    # Data
    for f in find-badfiles.sh install_rclone_service.sh install_rclone.sh install_snowsql.sh rclone_sync.sh storage-setup.sh y2j.sh; do
      [ -f "$repo_opt_bin/$f" ] && mv "$repo_opt_bin/$f" "$repo_opt_scripts/data/"
    done
    # Misc
    for f in agm.disable_sh antigravity_arm.sh tmuxinator.zsh toggle_browser.scpt; do
      [ -f "$repo_opt_bin/$f" ] && mv "$repo_opt_bin/$f" "$repo_opt_scripts/misc/"
    done
    
    [ -f "$repo_opt_bin/README.md" ] && rm "$repo_opt_bin/README.md"
  fi
}

migrate_opt_bin
git config --global pager.branch false
git config --global push.default current

[ ! -d "${HOME}/git" ] && mkdir -p "${HOME}/git"

# skip login check for sshd 
touch "${HOME}/.gitenv.nologin"

# vim
curl -fsSLo ~/.vim/autoload/plug.vim --create-dirs https://raw.githubusercontent.com/junegunn/vim-plug/master/plug.vim
vim +'PlugInstall --sync' +qall

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
    
    # Set Chromium as default browser
    if command -v apt-get &> /dev/null; then
      echo "Ensuring Chromium is installed and set as default..."
      sudo apt-get install -y -qq chromium-browser
      
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
    [[ "$filename" == "requirements.txt" ]] && continue
    [[ "$filename" == "GEMINI.md" ]] && continue
    
    echo "Creating symlink to $file in home directory."
    ln -sf "${file}" "${HOME}/${filename}"
done

# force a few
for file in ".profile" ".zshrc" ".bash_logout" ".bashrc"; do
  ln -sf "${BASE_DIR}/opt/profiles/${file}" "${HOME}/${file}"
done 

# Gemini CLI Configuration (Skills and Policies)
if [ -f "${BASE_DIR}/opt/scripts/system/install_gemini_skills.sh" ]; then
    "${BASE_DIR}/opt/scripts/system/install_gemini_skills.sh"
fi

NIX_MANAGED_FILE="${HOME}/.config/nix_managed"

if [ -f "$NIX_MANAGED_FILE" ]; then
  echo "Skipping apt-get because the env is managed with nix, found $NIX_MANAGED_FILE"
else
  if command -v apt-get &> /dev/null; then
    sudo apt-get install -y -qq \
      build-essential \
      make \
      corkscrew \
      htop \
      iputils-ping \
      jq \
      lsof \
      net-tools \
      psmisc \
      zsh \
      protobuf-compiler
    
    # Set zsh as default shell if not already
    if [ "$SHELL" != "$(which zsh)" ]; then
      echo "Changing default shell to zsh..."
      sudo chsh -s "$(which zsh)" "$USER"
    fi
  fi
fi

# yum script
if [ -f "$NIX_MANAGED_FILE" ]; then
  echo "Skipping yum because the env is managed with nix, found $NIX_MANAGED_FILE"
else
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
fi

# brew script for macOS
if [[ "$(uname -s)" == "Darwin" ]]; then
  if command -v brew &> /dev/null; then
    echo "Detected macOS. Running brew bundle..."
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

# only setup these scripts when docker is installed and responsive
if command -v docker &> /dev/null; then
    # Setup docker permissions for the current user
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
export PATH="${HOME}/.goenv/bin:${PATH}"
if command -v goenv &> /dev/null; then
  eval "$(goenv init -)"
  echo "Ensuring Go latest is installed..."
  goenv install -s latest
  # Set the latest installed version as global
  LATEST_INSTALLED=$(goenv versions --bare | tail -1)
  if [ -n "$LATEST_INSTALLED" ]; then
    goenv global "$LATEST_INSTALLED"
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

# build and install tmux-mgr
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
