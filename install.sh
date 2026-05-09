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
    if [ -f "${BASE_DIR}/opt/bin/setup_jtop.sh" ]; then
      echo "Setting up jtop and jetson-stats..."
      "${BASE_DIR}/opt/bin/setup_jtop.sh"
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
    echo "Creating symlink to $file in home directory."
    ln -sf "${file}" "${HOME}/$(basename "${file}")"
done

# force a few
for file in ".profile" ".zshrc" ".bash_logout" ".bashrc"; do
  ln -sf "${BASE_DIR}/opt/profiles/${file}" "${HOME}/${file}"
done 

# Gemini CLI Configuration (Skills and Policies)
if [ -f "${BASE_DIR}/opt/bin/install_gemini_skills.sh" ]; then
    "${BASE_DIR}/opt/bin/install_gemini_skills.sh"
fi

NIX_MANAGED_FILE="${HOME}/.config/nix_managed"

if [ -f "$NIX_MANAGED_FILE" ]; then
  echo "Skipping apt-get because the env is managed with nix, found $NIX_MANAGED_FILE"
else
  if command -v apt-get &> /dev/null; then
    sudo apt-get install -y -qq \
      corkscrew \
      htop \
      iputils-ping \
      jq \
      lsof \
      net-tools \
      psmisc \
      zsh
    
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
    if [ -f "${BASE_DIR}/opt/bin/vault-setup.sh" ]; then
      "${BASE_DIR}/opt/bin/vault-setup.sh"
    fi
  else
    echo "WARNING: Homebrew not found. Please install it: https://brew.sh/"
  fi
fi

# only setup these scripts when docker is installed and responsive
if command -v docker &> /dev/null; then
    # Setup docker permissions for the current user
    "${HOME}/opt/bin/setup_docker_perms.sh"
    
    # Check if Docker daemon is actually running (timeout after 5 seconds)
    # Use gtimeout on macOS (coreutils), timeout on Linux
    TIMEOUT_CMD="timeout"
    command -v timeout &>/dev/null || TIMEOUT_CMD="gtimeout"
    if ($TIMEOUT_CMD 5 docker info &>/dev/null 2>&1) 2>/dev/null; then
        [ ! -f "${HOME}/.ruby.env" ] && source "${HOME}/opt/bin/setup_ruby-docker.sh"
        [ ! -f "${HOME}/.dindcenv" ] && source "${HOME}/opt/bin/setup_dindc_alias.sh"
    else
        echo "NOTE: Docker is installed but the daemon is not running. Skipping Docker-dependent setup."
    fi
fi

# don't bother installing without corkscrew
if command -v corkscrew &> /dev/null; then
    [ ! -f "${HOME}/.gitenv" ] && source "${HOME}/opt/bin/setup_git_alias.sh"
fi

# install goenv
if ! command -v goenv &> /dev/null; then
  echo "Installing goenv..."
  if [[ "$(uname -s)" == "Darwin" ]] && command -v brew &> /dev/null; then
    brew install goenv
  else
    # Linux / Jetson / others
    if [ ! -d "${HOME}/.goenv" ]; then
      git clone https://github.com/syndbg/goenv.git "${HOME}/.goenv"
    else
      (cd "${HOME}/.goenv" && git pull)
    fi
  fi
  
  # Initialize goenv to install a version if none exists
  export PATH="${HOME}/.goenv/bin:${PATH}"
  if command -v goenv &> /dev/null; then
    eval "$(goenv init -)"
    if [ -z "$(goenv versions --bare)" ]; then
        echo "No Go versions detected. Installing latest..."
        goenv install latest
        goenv global $(goenv versions --bare | tail -1)
    fi
  fi
fi

# install pyenv
if ! command -v pyenv &> /dev/null; then
  echo "Installing pyenv..."
  if [[ "$(uname -s)" == "Darwin" ]] && command -v brew &> /dev/null; then
    brew install pyenv
  else
    if [ ! -d "${HOME}/.pyenv" ]; then
      git clone https://github.com/pyenv/pyenv.git "${HOME}/.pyenv"
    else
      (cd "${HOME}/.pyenv" && git pull)
    fi
  fi
fi

# install rbenv
if ! command -v rbenv &> /dev/null; then
  echo "Installing rbenv..."
  if [[ "$(uname -s)" == "Darwin" ]] && command -v brew &> /dev/null; then
    brew install rbenv
  else
    if [ ! -d "${HOME}/.rbenv" ]; then
      git clone https://github.com/rbenv/rbenv.git "${HOME}/.rbenv"
      # Also need ruby-build plugin for rbenv
      mkdir -p "${HOME}/.rbenv/plugins"
      git clone https://github.com/rbenv/ruby-build.git "${HOME}/.rbenv/plugins/ruby-build"
    else
      (cd "${HOME}/.rbenv" && git pull)
    fi
  fi
fi

# install nvm
if [ ! -d "${HOME}/.nvm" ]; then
  echo "Installing nvm..."
  curl -fsSL -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash > /dev/null 2>&1
fi

# install Gemini CLI
if [ -f "${BASE_DIR}/opt/bin/gemini_install.sh" ]; then
    echo "Installing Gemini CLI..."
    "${BASE_DIR}/opt/bin/gemini_install.sh"
fi

# build and install tmux-mgr
if [ -f "${BASE_DIR}/src/tmux-mgr/build.sh" ]; then
    echo "Installing tmux-mgr..."
    "${BASE_DIR}/src/tmux-mgr/build.sh"
fi

# install fnm
if ! command -v fnm &> /dev/null; then
  echo "Installing fnm..."
  curl -fsSL https://fnm.vercel.app/install | bash -s -- --skip-shell > /dev/null 2>&1
fi

# setup sshd server if requested
if [ -f "${HOME}/.sshd.env" ]; then
    "${HOME}/opt/bin/sshd_run.sh"
fi

if [ -f "${HOME}/.gitrepos" ] ; then
  cd "${HOME}"
  "${HOME}/.gitrepos"
fi
