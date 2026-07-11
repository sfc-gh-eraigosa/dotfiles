#!/bin/bash
#
function install_zsh_centos7() {
    sudo yum update -y
    sudo yum install -y git make ncurses-devel gcc autoconf man
    git clone -b zsh-5.7.1 https://github.com/zsh-users/zsh.git /tmp/zsh
    (
        cd /tmp/zsh || exit 1
        ./Util/preconfig
        ./configure
        sudo make -j 20 install.bin install.modules install.fns
    )
}

BASE_DIR="$(cd "$(dirname "$0")" && pwd)"
export BASE_DIR

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

while IFS= read -r file; do
    filename=$(basename "$file")
    # Skip metadata and non-profile files
    [[ "$filename" == "Brewfile" ]] && continue
    [[ "$filename" == "packages.tsv" ]] && continue
    [[ "$filename" == "requirements.txt" ]] && continue
    [[ "$filename" == "AGENTS.md" ]] && continue
    [[ "$filename" == "CLAUDE.md" ]] && continue

    echo "Creating symlink to $file in home directory."
    ln -sf "${file}" "${HOME}/${filename}"
done < <(find "${BASE_DIR}/opt/profiles" -type f)

# force a few
for file in ".profile" ".zshenv" ".zshrc" ".bash_logout" ".bashrc"; do
  ln -sf "${BASE_DIR}/opt/profiles/${file}" "${HOME}/${file}"
done 

# Shared skill sync — links every SKILL.md into BOTH ~/.gemini/config/skills
# (Antigravity) and ~/.claude/skills (Claude). Single source of truth for both
# assistants.
if [ -f "${BASE_DIR}/opt/scripts/system/sync-skills.sh" ]; then
    bash "${BASE_DIR}/opt/scripts/system/sync-skills.sh"
fi

# Antigravity CLI Configuration (Hooks, Aliases, legacy Gemini cleanup)
if [ -f "${BASE_DIR}/opt/scripts/system/install_antigravity_skills.sh" ]; then
    # sync-skills handles the skill links now; this only does Antigravity-specific config.
    "${BASE_DIR}/opt/scripts/system/install_antigravity_skills.sh"
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

# Install yq (YAML processor). Same rationale as sops: macOS gets the mikefarah
# build from packages.tsv (brew); Linux/WSL fetches the official binary because
# the apt `yq` is the incompatible kislyuk variant. Needed by sync-plugins.sh.
if [ -f "${BASE_DIR}/opt/scripts/system/install_yq.sh" ]; then
    echo "Installing yq..."
    "${BASE_DIR}/opt/scripts/system/install_yq.sh" || echo "WARNING: yq install reported problems; continuing."
fi

# Install the Snowflake CLI (`snow`). Replaces the old .zshrc daily-maintenance
# pip auto-install, which broke on PEP 668 (externally-managed-environment)
# systems. macOS uses the homebrew-core formula; Linux uses pipx so the system
# Python is untouched.
if [ -f "${BASE_DIR}/opt/scripts/system/install_snowflake_cli.sh" ]; then
    echo "Installing snowflake-cli..."
    "${BASE_DIR}/opt/scripts/system/install_snowflake_cli.sh" || echo "WARNING: snowflake-cli install reported problems; continuing."
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
  # Pass the shell to `goenv init` EXPLICITLY. Bare `goenv init -` infers the
  # shell from $SHELL, so on a host whose login shell is zsh it emits zsh code
  # (a `while IFS=: read -rA …` PATH loop) even though this script runs under
  # bash — bash then errors `read: -A: invalid option`, the loop's `_NEW_PATH`
  # stays empty, and `export PATH="$_NEW_PATH"` wipes PATH, after which every
  # coreutil (tr, uname, git…) is "command not found" and the install collapses.
  # `goenv init - bash` forces bash-safe output. The guard below is a belt-and-
  # suspenders backstop: if any goenv build still drops the system bin dirs,
  # restore a known-good PATH. See docs/mbo/specs/shell-portability.md.
  __goenv_path_safe="${PATH}"
  eval "$(goenv init - bash)"
  case ":${PATH}:" in
    *":/usr/bin:"*) : ;;                          # system PATH survived
    *) PATH="${PATH}:${__goenv_path_safe}" ;;     # init clobbered PATH; restore
  esac
  export PATH
  unset __goenv_path_safe
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

# install Antigravity CLI (agy) — Gemini CLI's successor (Gemini CLI EOL 2026-06-18)
if [ -f "${BASE_DIR}/opt/scripts/system/antigravity_install.sh" ]; then
    echo "Installing Antigravity CLI..."
    "${BASE_DIR}/opt/scripts/system/antigravity_install.sh"
fi

# Retired Gemini CLI leftovers: consent-based teardown (prompts when leftovers
# are found; --keep marker suppresses the ask forever; no-op in CI/non-TTY).
if [ -f "${BASE_DIR}/opt/scripts/system/gemini_teardown.sh" ]; then
    "${BASE_DIR}/opt/scripts/system/gemini_teardown.sh" || echo "WARNING: gemini teardown reported problems; continuing."
fi

# Google CLI (Antigravity & Workspace) Setup
if [ -f "${BASE_DIR}/opt/scripts/system/google-cli-setup.sh" ]; then
    # This configures gws for BOTH Antigravity CLI and Claude Code via shared skills.
    echo "Setting up Google CLI (Antigravity & Workspace)..."
    "${BASE_DIR}/opt/scripts/system/google-cli-setup.sh"
fi

# Antigravity hooks are provisioned by install_antigravity_skills.sh (called
# above): guard scripts are copied into ~/.gemini/config/hooks/ and the wiring
# is rendered to ~/.gemini/config/hooks.json. agy owns its own settings file
# (~/.gemini/antigravity-cli/settings.json) — nothing to merge or symlink here.

# install Claude Code CLI (macOS via brew cask, Linux/WSL via npm)
if [ -f "${BASE_DIR}/opt/scripts/system/claude_install.sh" ]; then
    echo "Installing Claude Code CLI..."
    "${BASE_DIR}/opt/scripts/system/claude_install.sh"
fi

# Claude settings + hooks are provisioned by install_claude_skills.sh (called
# above): ~/.claude/settings.json is a host-owned file with the forced subset
# (hooks, statusLine, deny/ask) merged in, and hooks are copied into
# ~/.claude/hooks/. No symlink and no repo-internal host copy here.

# Sync AI plugins from the manifest (ai/plugins.yaml). Ensure-only: installs +
# enables the listed plugins; never removes anything. Runs after the Claude CLI
# (claude_install.sh) and yq are installed.
if [ -f "${BASE_DIR}/opt/scripts/system/sync-plugins.sh" ]; then
    echo "Syncing AI plugins..."
    "${BASE_DIR}/opt/scripts/system/sync-plugins.sh" || echo "WARNING: plugin sync reported problems; continuing."
fi

# Install AI teams: transform ai/teams personas into native agents for Claude,
# Antigravity, and Ollama. Runs after yq + the assistant configs. Validates the source
# first; each tool emit degrades gracefully, so a teams problem never aborts bootstrap.
if [ -f "${BASE_DIR}/opt/scripts/system/install_ai_teams.sh" ]; then
    echo "Installing AI teams..."
    "${BASE_DIR}/opt/scripts/system/install_ai_teams.sh" || echo "WARNING: AI teams install reported problems; continuing."
fi

# build and install gss
if [ -f "${BASE_DIR}/sdk/gss/build.sh" ]; then
    echo "Installing gss (dotfiles manager)..."
    bash "${BASE_DIR}/sdk/gss/build.sh"
    if [ -f "${HOME}/opt/bin/gss" ]; then
        echo "--------------------------------------------------"
        "${HOME}/opt/bin/gss" version
        echo "--------------------------------------------------"
    fi
fi

# build and install tmux-mg
if [ -f "${BASE_DIR}/sdk/tmux-mgr/build.sh" ]; then
    echo "Installing tmux-mgr..."
    bash "${BASE_DIR}/sdk/tmux-mgr/build.sh"
    if [ -f "${HOME}/opt/bin/tmux-mgr" ]; then
        echo "--------------------------------------------------"
        "${HOME}/opt/bin/tmux-mgr" version
        echo "--------------------------------------------------"
        # install aliases
        "${HOME}/opt/bin/tmux-mgr" alias install
    fi
fi

# build and install wol
if [ -f "${BASE_DIR}/sdk/wol/build.sh" ]; then
    echo "Installing wol (Wake-on-LAN utility)..."
    bash "${BASE_DIR}/sdk/wol/build.sh"
    if [ -f "${HOME}/opt/bin/wol" ]; then
        echo "--------------------------------------------------"
        "${HOME}/opt/bin/wol" version
        echo "--------------------------------------------------"
    fi
fi

# build and install gsl
if [ -f "${BASE_DIR}/sdk/gsl/build.sh" ]; then
    echo "Installing gsl (Go status line)..."
    bash "${BASE_DIR}/sdk/gsl/build.sh"
    if [ -f "${HOME}/opt/bin/gsl" ]; then
        echo "--------------------------------------------------"
        "${HOME}/opt/bin/gsl" version
        echo "--------------------------------------------------"
    fi
fi

# Configure the Nerd Font (MesloLGS Nerd Font) used by gsl's powerline style.
# Runs AFTER the gsl build so both the gsl skill files (linked by sync-skills
# above) and the freshly-built ~/opt/bin/gsl exist. OS-dispatch to the
# gsl-packaged installers under sdk/gsl/scripts/.
GSL_FONT_SCRIPTS="${BASE_DIR}/sdk/gsl/scripts"
case "$(uname -s)" in
  Darwin)
    if [ -f "${GSL_FONT_SCRIPTS}/install_nerd_font_macos.sh" ]; then
      echo "Configuring Nerd Font (macOS)..."
      bash "${GSL_FONT_SCRIPTS}/install_nerd_font_macos.sh" || \
        echo "WARNING: macOS Nerd Font setup reported problems; continuing."
    fi
    ;;
  Linux)
    if [ ! -f "$NIX_MANAGED_FILE" ] && [ -f "${GSL_FONT_SCRIPTS}/install_nerd_font_linux.sh" ]; then
      echo "Configuring Nerd Font (Linux/WSL)..."
      bash "${GSL_FONT_SCRIPTS}/install_nerd_font_linux.sh" || \
        echo "WARNING: Linux Nerd Font setup reported problems; continuing."
    fi
    ;;
esac

# Prove the installed font covers every codepoint gsl renders (non-fatal).
if [ -f "${GSL_FONT_SCRIPTS}/check-font-glyphs.sh" ] && command -v go >/dev/null 2>&1; then
  echo "Validating gsl glyph coverage..."
  bash "${GSL_FONT_SCRIPTS}/check-font-glyphs.sh" || \
    echo "WARNING: glyph-coverage check failed; gsl powerline glyphs may not render."
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
  cd "${HOME}" || exit 1
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

# Final reminder (WSL): if the interactive Windows setup ran this invocation, the
# one thing that can't be scripted is Wispr Flow's shortcuts — surface it last so
# it isn't scrolled away by earlier output. install_windows.sh sets this marker.
WIN_SETUP_MARKER="${HOME}/.config/dotfiles/.windows-setup-just-ran"
if [ -f "$WIN_SETUP_MARKER" ]; then
  rm -f "$WIN_SETUP_MARKER" 2>/dev/null || true
  _b=$'\033[1m'; _x=$'\033[0m'
  if [ -t 1 ]; then
    # Soft pastel-rainbow palette (xterm-256) — friendly, not alarming.
    _hues=(210 216 222 157 117 147 183 219)
    _sky=$'\033[38;5;117m'; _mint=$'\033[38;5;157m'; _dim=$'\033[38;5;245m'
    # Render an ASCII string with a flowing pastel rainbow at phase $2 (no newline).
    _rainbow() {
      local s="$1" phase="${2:-0}" i out=""
      for ((i=0; i<${#s}; i++)); do
        out+=$'\033[38;5;'"${_hues[$(((i+phase) % ${#_hues[@]}))]}"'m'"${s:i:1}"
      done
      printf '%s%s' "$out" "$_x"
    }
    _rule="------------------------------------------------------------------------"
    _title="All set!  Just one quick Wispr Flow step to finish up"
    printf '\n'
    # Gentle flowing-rainbow animation on the title (~0.5s; skipped if no sleep).
    for _p in 0 1 2 3 4 5 6 7; do
      printf '\r  🌈 %s 🌈' "$(_rainbow "$_title" "$_p")"
      sleep 0.06 2>/dev/null || true
    done
    printf '\n'
    printf '%s\n' "$(_rainbow "$_rule" 0)"
    printf '%s\n' "  In ${_b}Wispr Flow${_x} → ${_sky}Settings › General › Shortcuts${_x}, set ${_b}all three${_x} shortcuts"
    printf '%s\n' "  off the Win key (Flow's ${_b}Ctrl+Win${_x} default just overlaps the macOS hotkeys):"
    printf '\n'
    printf '%s\n' "    ${_mint}♪${_x} ${_b}Push-to-talk${_x} : any non-Win combo  (e.g. ${_sky}Ctrl+Shift+F12${_x})"
    printf '%s\n' "    ${_mint}♪${_x} ${_b}Hands-free${_x}   : any non-Win combo  (e.g. ${_sky}Ctrl+Shift+F11${_x})"
    printf '%s\n' "    ${_mint}♪${_x} ${_b}Command mode${_x} : any non-Win combo  (e.g. ${_sky}Ctrl+Shift+F10${_x})"
    printf '\n'
    printf '%s\n' "  ${_dim}The Copilot key itself needs no Flow shortcut — PowerToys + macos.ahk drive it.${_x}"
    printf '%s\n' "  ${_dim}This step is manual — Flow keeps its settings in a binary, cloud-synced store.${_x}"
    printf '%s\n' "  ${_dim}Full guide:${_x} ${_sky}<Desktop>\\Apps\\scripts\\WISPR-FLOW.md${_x}"
    printf '\n'
    printf '%s\n' "  ${_mint}♪${_x} ${_b}Dictation starts OFF${_x} — press ${_sky}F10${_x} once to turn the dictation toggle ${_b}ON${_x}."
    printf '%s\n' "  ${_dim}The AutoHotkey setup (macos.ahk) powers a hotkey-automation workflow: extra${_x}"
    printf '%s\n' "  ${_dim}trigger keys via ${_x}${_sky}F9${_x}${_dim}, calibrate via ${_x}${_sky}F11${_x}${_dim}, hold ${_x}${_sky}F1${_x}${_dim} for help — and it overrides${_x}"
    printf '%s\n' "  ${_dim}Flow's built-in hands-free mode so the Copilot key drives dictation instead.${_x}"
    printf '%s\n' "$(_rainbow "$_rule" 4)"
    printf '\n'
    unset -f _rainbow; unset _hues _sky _mint _dim _p _rule _title
  else
    cat <<'BANNER'

------------------------------------------------------------------------
  All set! Just one quick Wispr Flow step to finish up.
------------------------------------------------------------------------
  In Wispr Flow -> Settings > General > Shortcuts, set all three shortcuts
  off the Win key (Flow's Ctrl+Win default just overlaps the macOS hotkeys):

    - Push-to-talk : any non-Win combo  (e.g. Ctrl+Shift+F12)
    - Hands-free   : any non-Win combo  (e.g. Ctrl+Shift+F11)
    - Command mode : any non-Win combo  (e.g. Ctrl+Shift+F10)

  The Copilot key itself needs no Flow shortcut - PowerToys + macos.ahk drive it.
  This step is manual - Flow keeps its settings in a binary, cloud-synced store.
  Full guide: <Desktop>\Apps\scripts\WISPR-FLOW.md

  - Dictation starts OFF - press F10 once to turn the dictation toggle ON.
  The AutoHotkey setup (macos.ahk) powers a hotkey-automation workflow: extra
  trigger keys via F9, calibrate via F11, hold F1 for help - and it overrides
  Flow's built-in hands-free mode so the Copilot key drives dictation instead.
------------------------------------------------------------------------
BANNER
  fi
  unset _b _x
fi
