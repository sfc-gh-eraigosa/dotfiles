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

# --- Self-sufficient PATH -----------------------------------------------------
# This script INSTALLS into ~/opt/bin (sops, yq, kubectl, helm, kind, and every
# sdk/ binary) and ~/.local/bin (pipx CLIs, agy, claude) and then immediately
# CONSUMES those tools — install_ai_teams.sh and sync-plugins.sh both hard-require
# `yq`, the gff early-export below probes `command -v gff`.
#
# Those directories are only added to PATH by ~/.profile, which is sourced by
# LOGIN shells. `fleet update <host>` runs install.sh over `ssh -t host "...
# ./install.sh"` — a NON-login shell — so PATH lacked both, and the run failed
# with a cascade of "installed but not resolvable" errors ("yq not resolvable
# after install", "sync-plugins: 'yq' not found", "install_ai_teams: yq is
# required"). Make the script independent of the caller's shell startup files.
for _ip_dir in "${HOME}/opt/bin" "${HOME}/.local/bin"; do
  case ":${PATH}:" in
    *":${_ip_dir}:"*) ;;                    # already present — don't duplicate
    *) PATH="${_ip_dir}:${PATH}" ;;
  esac
done
unset _ip_dir
export PATH

# gff_on is env-only and fail-open; it must exist before the FIRST gate. Sourcing
# it here (not at the bootstrap point) is load-bearing: a gate that calls an
# undefined gff_on gets exit 127, takes the else branch, and SKIPS the step —
# failing CLOSED on exactly the fresh machines gff must be harmless on.
. "${BASE_DIR}/opt/lib/gff.sh"

# Early flag export (fail-open): pre-bootstrap gates read env only. If a gff
# binary already exists (any run after the first), materialize the overrides
# now so install.system/shell/pkg/tools/windows gates honor them. The
# mid-script bootstrap below remains the authoritative refresh.
if command -v gff >/dev/null 2>&1; then
  set -a
  eval "$(cd "${BASE_DIR}" && gff export --shell 2>/dev/null || true)"
  set +a
fi

# --- Build-phase gating (Docker layer caching; no effect on real machines) --
# `--phase deps|config` lets a CONTAINER build run the repo-INDEPENDENT work
# (runtime toolchains, external tools, OS packages) in a cache-friendly EARLY
# image layer, and the repo-DEPENDENT work (shell profiles, AI skills, sdk
# builds) in a LATER layer — so a source change only re-runs config. Default
# `all` applies NO overrides, so a normal `./install.sh` is byte-for-byte the
# same as before. Implemented purely by toggling the same GFF_* env that
# gff_on already reads, so there are zero per-section changes below. Because
# both gff `export` blocks (above and at the bootstrap) can overwrite GFF_*,
# apply_install_phase is re-invoked after each of them.
INSTALL_PHASE=all
_ip_prev=""
for _ip_arg in "$@"; do
  case "$_ip_arg" in
    --phase=*) INSTALL_PHASE="${_ip_arg#--phase=}" ;;
  esac
  [ "$_ip_prev" = "--phase" ] && INSTALL_PHASE="$_ip_arg"
  _ip_prev="$_ip_arg"
done
unset _ip_prev _ip_arg

# Repo-DEPENDENT (config) vs repo-INDEPENDENT (deps) gff keys, sans GFF_ prefix.
#
# ADDING A NEW gff-gated section below? Put its key in EXACTLY ONE list here, or
# the Docker build's deps/config layer split silently regresses (see AGENTS.md
# "Docker build layering", and the two-phase Dockerfile). Rule of thumb:
#   - EXTERNAL INSTALL DEPENDENCY (apt/brew package, a downloaded tool/CLI, a
#     language runtime/toolchain) -> _IP_DEPS_FLAGS. Runs in the CACHED deps
#     layer; omitting it makes the step re-run on every commit (slow).
#   - CONFIG / SKILL / SYMLINK / any repo-content step -> _IP_CONFIG_FLAGS. Runs
#     in the per-commit config layer; omitting it BAKES the step into the cached
#     deps layer, so later edits to it stop taking effect per commit (a bug).
_IP_CONFIG_FLAGS="INSTALL_SHELL_PROFILES INSTALL_SHELL_DEFAULT_ZSH INSTALL_DESKTOP_GNOME_KEYS INSTALL_AI_SKILLS INSTALL_AI_ANTIGRAVITY INSTALL_AI_CLAUDE INSTALL_TOOLS_GIT_ALIASES INSTALL_TOOLS_HERDR_INTEGRATIONS INSTALL_TOOLS_HERDR_CONFIG INSTALL_SDK_GSS INSTALL_SDK_TMUX_MGR INSTALL_SDK_WOL INSTALL_SDK_GSL INSTALL_SDK_GFF"
_IP_DEPS_FLAGS="INSTALL_PKG_COMMON_CORE INSTALL_PKG_BREWFILE INSTALL_TOOLS_SOPS INSTALL_TOOLS_YQ INSTALL_TOOLS_K8S INSTALL_TOOLS_HERDR INSTALL_TOOLS_SNOWFLAKE INSTALL_TOOLS_DOCKER INSTALL_RUNTIME_GOENV INSTALL_RUNTIME_PYENV INSTALL_RUNTIME_RBENV INSTALL_RUNTIME_NVM INSTALL_SHELL_OH_MY_ZSH_UPDATE"
apply_install_phase() {
  case "$INSTALL_PHASE" in
    deps)   for _f in $_IP_CONFIG_FLAGS; do export "GFF_${_f}=false"; done ;;
    config) for _f in $_IP_DEPS_FLAGS;   do export "GFF_${_f}=false"; done ;;
    all|"") : ;;
    *) echo "WARNING: unknown --phase '${INSTALL_PHASE}' (use deps|config|all); running as all"; INSTALL_PHASE=all ;;
  esac
}
[ "$INSTALL_PHASE" != "all" ] && echo "install.sh: build phase = ${INSTALL_PHASE}"
apply_install_phase

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

# Windows/WSL only — ASK phase: capture the y/n/s customization answer up front
# (all interactivity front-loaded). The deploy + PowerShell EXECUTION runs at
# the END of this script (--deferred), after the gff bootstrap has exported
# GFF_* — so install.windows.* overrides work with zero calling-shell steps.
if gff_on install.windows.desktop-deploy; then
  if [ -f "${BASE_DIR}/opt/bin/install_windows.sh" ]; then
    bash "${BASE_DIR}/opt/bin/install_windows.sh" "${BASE_DIR}" --ask
  fi
else gff_skip_msg install.windows.desktop-deploy; fi

# WSL only: keep the WSLInterop binfmt registration alive so Windows .exe
# interop survives (a wiped registration makes every .exe fail with
# "exec format error"; WSL's own self-heal unit is condition-blocked under
# WSL). No-op outside WSL; may prompt for sudo once.
if gff_on install.system.wsl-interop; then
  if [ -f "${BASE_DIR}/opt/scripts/system/wsl_interop_binfmt.sh" ]; then
    bash "${BASE_DIR}/opt/scripts/system/wsl_interop_binfmt.sh" || \
      echo "WARNING: WSL interop binfmt setup reported problems; continuing."
  fi
else gff_skip_msg install.system.wsl-interop; fi

# Source hardware detection
if gff_on install.system.jetson; then
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
      # DEBIAN_FRONTEND=noninteractive on the sudo env is load-bearing: a
      # debconf prompt (tzdata-class) blocks forever when this runs without a
      # tty — e.g. inside `docker build` (the Docker Image CI hang, PR #182).
      sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq chromium-browse
      
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
else gff_skip_msg install.system.jetson; fi

# NVIDIA DGX / discrete-GPU hosts (DGX Spark, workstation dGPU). Deliberately
# separate from the Jetson block above: a Jetson runs L4T and gets jtop, while
# a DGX Spark runs DGX OS on stock Ubuntu where jtop cannot work at all.
# setup_gpu.sh self-guards (no-ops on Jetson and on GPU-less hosts), so this
# stays inert everywhere else.
if gff_on install.system.gpu; then
  if [ -f "${BASE_DIR}/opt/scripts/system/setup_gpu.sh" ]; then
    bash "${BASE_DIR}/opt/scripts/system/setup_gpu.sh" ||
      echo "WARNING: GPU monitoring setup reported problems; continuing."
  fi
else gff_skip_msg install.system.gpu; fi

if gff_on install.shell.profiles; then
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
for file in ".profile" ".zprofile" ".zshenv" ".zshrc" ".bash_logout" ".bashrc"; do
  ln -sf "${BASE_DIR}/opt/profiles/${file}" "${HOME}/${file}"
done
else gff_skip_msg install.shell.profiles; fi

# macOS-style keyboard layout on Linux — the counterpart to the Cmd-key mapping
# macos.ahk applies on Windows, so one set of muscle memory works on every
# machine. Two halves, both self-guarding (a no-op off a real desktop session, so
# CI, docker, WSL, plain SSH and macOS are unaffected):
#   1. gnome-desktop-defaults.sh — gsettings: the desktop-level ACTIONS
#      (Cmd+Space/Tab/M/H, an inert lone-Cmd tap).
#   2. macos-keys-linux.sh — keyd: the in-application EDITING keys, which is what
#      makes Cmd+C work in Firefox/VS Code/Nautilus and not just the terminal.
# Both sit under keyboard.macos.enabled, the one switch that turns macOS-style
# keyboard customization off on EVERY OS (see the README section of the same name).
if gff_on keyboard.macos.enabled; then

  if gff_on install.desktop.gnome-keys; then
  if [ -x "${BASE_DIR}/opt/scripts/system/gnome-desktop-defaults.sh" ]; then
    "${BASE_DIR}/opt/scripts/system/gnome-desktop-defaults.sh" || echo "WARNING: GNOME desktop defaults reported problems; continuing."
  fi
  else gff_skip_msg install.desktop.gnome-keys; fi

  if gff_on install.desktop.macos-keys; then
  if [ -x "${BASE_DIR}/opt/scripts/system/macos-keys-linux.sh" ]; then
    "${BASE_DIR}/opt/scripts/system/macos-keys-linux.sh" || echo "WARNING: macOS key mappings reported problems; continuing."
  fi
  else gff_skip_msg install.desktop.macos-keys; fi

else gff_skip_msg keyboard.macos.enabled; fi

# Shared skill sync — links every SKILL.md into BOTH ~/.gemini/config/skills
# (Antigravity) and ~/.claude/skills (Claude). Single source of truth for both
# assistants.
if gff_on install.ai.skills; then
  if [ -f "${BASE_DIR}/opt/scripts/system/sync-skills.sh" ]; then
    bash "${BASE_DIR}/opt/scripts/system/sync-skills.sh"
  fi
else gff_skip_msg install.ai.skills; fi

# Antigravity CLI Configuration (Hooks, Aliases, legacy Gemini cleanup)
if gff_on install.ai.antigravity; then
  if [ -f "${BASE_DIR}/opt/scripts/system/install_antigravity_skills.sh" ]; then
    # sync-skills handles the skill links now; this only does Antigravity-specific config.
    "${BASE_DIR}/opt/scripts/system/install_antigravity_skills.sh"
  fi
else gff_skip_msg install.ai.antigravity; fi

# Claude Code Configuration (Settings, Commands, Hooks, Aliases)
if gff_on install.ai.claude; then
  if [ -f "${BASE_DIR}/opt/scripts/system/install_claude_skills.sh" ]; then
    # sync-skills handles the skill links now; this only does Claude-specific config.
    "${BASE_DIR}/opt/scripts/system/install_claude_skills.sh"
  fi
else gff_skip_msg install.ai.claude; fi

# Git privacy hooks (global core.hooksPath): judge staged content, commit
# messages and outgoing commits WHATEVER wrote them — the layer behind the
# agent-side privacy_guard, which only sees tool calls. Same rule library
# (ai/hooks/privacy_rules.sh); chains to any repo-local hook.
if gff_on install.git.hooks; then
  if [ -f "${BASE_DIR}/opt/scripts/git/install_git_hooks.sh" ]; then
    "${BASE_DIR}/opt/scripts/git/install_git_hooks.sh"
  fi
else gff_skip_msg install.git.hooks; fi

# gitleaks: the broad, upstream-maintained secret ruleset the privacy guard
# (agent hook + git hooks) judges with when the binary is present; our built-in
# shapes stay as the floor. Flag off => install nothing AND tell the hooks to
# skip it (marker file), so a binary from elsewhere does not re-enable it.
# Every judged call is timed; `make hook-timing` reports and goes red over budget.
if gff_on install.git.gitleaks; then
  if [ -f "${BASE_DIR}/opt/scripts/git/install_gitleaks.sh" ]; then
    "${BASE_DIR}/opt/scripts/git/install_gitleaks.sh" || echo "WARN: gitleaks install failed; the privacy guard keeps its built-in secret shapes"
  fi
else
  gff_skip_msg install.git.gitleaks
  [ -f "${BASE_DIR}/opt/scripts/git/install_gitleaks.sh" ] && "${BASE_DIR}/opt/scripts/git/install_gitleaks.sh" --off
fi

# herdr agent integrations (`herdr integration install claude|antigravity-cli`):
# the hook scripts that report each agent's working/blocked/done state to the
# herdr sidebar. install_antigravity_skills.sh MERGES its `guards` entry into
# ~/.gemini/config/hooks.json (agy-parity unit 4), so herdr's entry survives
# either ordering; running after it just keeps the sequence readable. Only
# integrations whose agent CLI is present are installed; the binary itself is
# the deps-phase install.tools.herdr block.
if gff_on install.tools.herdr-integrations; then
  if [ -f "${BASE_DIR}/opt/scripts/system/install_herdr.sh" ]; then
    echo "Installing herdr agent integrations..."
    "${BASE_DIR}/opt/scripts/system/install_herdr.sh" integrations || echo "WARNING: herdr integrations reported problems; continuing."
  fi
else gff_skip_msg install.tools.herdr-integrations; fi

# herdr managed config (~/.config/herdr/config.toml): herdr paints its own
# sidebar/panel colors, and its default dark catppuccin theme is unreadable on
# a Solarized Light terminal profile. The rendered template turns on herdr's
# host light/dark following with the fleet Solarized pair, so the right palette
# is picked per terminal profile at runtime. The host owns the file: it is only
# rewritten while it carries the "managed by dotfiles" marker.
if gff_on install.tools.herdr-config; then
  if [ -f "${BASE_DIR}/opt/scripts/system/install_herdr.sh" ]; then
    echo "Installing herdr config..."
    "${BASE_DIR}/opt/scripts/system/install_herdr.sh" config || echo "WARNING: herdr config reported problems; continuing."
  fi
else gff_skip_msg install.tools.herdr-config; fi

NIX_MANAGED_FILE="${HOME}/.config/nix_managed"

if [ -f "$NIX_MANAGED_FILE" ]; then
  echo "Skipping system package install because the env is managed with nix, found $NIX_MANAGED_FILE"
else
  # Install the curated common-core packages. They are defined once in
  # opt/profiles/packages.tsv; the per-platform installers in opt/bin translate
  # that list to the right package manager (apt on Debian/Ubuntu/WSL, Homebrew
  # on macOS). These can also be run by hand: `pkg-install` (auto-detect) o
  # `pkg-install-apt` / `pkg-install-brew` directly.
  if gff_on install.pkg.common-core; then
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
  else gff_skip_msg install.pkg.common-core; fi

  # Set zsh as the default shell on every platform — but only when zsh is
  # actually installed, so we never blank the login shell by passing an empty
  # path to chsh.
  if gff_on install.shell.default-zsh; then
  ZSH_PATH="$(command -v zsh || true)"
  if [ -n "$ZSH_PATH" ]; then
    # Compare against the PASSWD entry, not $SHELL. $SHELL is frozen at login,
    # so after a chsh it stays stale for the life of the session — testing it
    # re-runs `sudo chsh` on every install until the user logs out.
    # (macOS has no getent; fall back to $SHELL there.)
    if command -v getent &> /dev/null; then
      CURRENT_LOGIN_SHELL="$(getent passwd "${USER:-$(id -un)}" | cut -d: -f7)"
    else
      CURRENT_LOGIN_SHELL="$SHELL"
    fi
    if [ "$CURRENT_LOGIN_SHELL" != "$ZSH_PATH" ]; then
      echo "Changing default shell to zsh ($ZSH_PATH)..."
      # ${USER:-$(id -un)} so chsh still gets a real name in non-login/root shells.
      sudo chsh -s "$ZSH_PATH" "${USER:-$(id -un)}" || echo "WARNING: could not change default shell to zsh."
    fi

    # chsh only rewrites /etc/passwd. The running session keeps the old $SHELL,
    # and VTE/gnome-terminal prefers $SHELL OVER the passwd entry — so every new
    # terminal keeps opening the previous shell until the next logout. Push the
    # new value into the systemd user manager and the D-Bus activation env so
    # D-Bus-activated terminals (gnome-terminal-server) pick it up on their next
    # start. Best-effort: absent on macOS and in containers.
    if [ -n "${DBUS_SESSION_BUS_ADDRESS:-}" ]; then
      if command -v systemctl &> /dev/null; then
        systemctl --user set-environment "SHELL=${ZSH_PATH}" 2>/dev/null || true
      fi
      # Pass the VALUE explicitly. A bare `--systemd SHELL` re-reads $SHELL from
      # THIS process — still the OLD shell — and silently clobbers the line above.
      if command -v dbus-update-activation-environment &> /dev/null; then
        dbus-update-activation-environment --systemd "SHELL=${ZSH_PATH}" 2>/dev/null || true
      fi
    fi
  else
    echo "WARNING: zsh is not installed; leaving the default shell unchanged."
  fi
  else gff_skip_msg install.shell.default-zsh; fi
fi

# brew script for macOS: install the macOS-only extras from the Brewfile.
# (The cross-platform common core is installed above via pkg-install.)
if gff_on install.pkg.brewfile; then
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
else gff_skip_msg install.pkg.brewfile; fi

# Install sops (secrets management). macOS gets it from the Brewfile above;
# Linux/WSL has no usable apt package, so install_sops.sh fetches the official
# static release binary into ~/opt/bin. Safe to re-run on any platform.
if gff_on install.tools.sops; then
  if [ -f "${BASE_DIR}/opt/scripts/system/install_sops.sh" ]; then
    echo "Installing sops..."
    "${BASE_DIR}/opt/scripts/system/install_sops.sh" || echo "WARNING: sops install reported problems; continuing."
  fi
else gff_skip_msg install.tools.sops; fi

# Install yq (YAML processor). Same rationale as sops: macOS gets the mikefarah
# build from packages.tsv (brew); Linux/WSL fetches the official binary because
# the apt `yq` is the incompatible kislyuk variant. Needed by sync-plugins.sh.
if gff_on install.tools.yq; then
  if [ -f "${BASE_DIR}/opt/scripts/system/install_yq.sh" ]; then
    echo "Installing yq..."
    "${BASE_DIR}/opt/scripts/system/install_yq.sh" || echo "WARNING: yq install reported problems; continuing."
  fi
else gff_skip_msg install.tools.yq; fi

# Install the Kubernetes toolchain (kubectl, helm, kind). macOS gets them from
# packages.tsv (brew); Linux/WSL fetches the official release binaries because
# apt has no kind package and kubectl/helm apt repos lag upstream. Local k8s
# dev flows (kind cluster -> helm install -> kubectl) need all three.
if gff_on install.tools.k8s; then
  if [ -f "${BASE_DIR}/opt/scripts/system/install_k8s_tools.sh" ]; then
    echo "Installing Kubernetes toolchain (kubectl, helm, kind)..."
    "${BASE_DIR}/opt/scripts/system/install_k8s_tools.sh" || echo "WARNING: k8s toolchain install reported problems; continuing."
  fi
else gff_skip_msg install.tools.k8s; fi

# Install herdr (terminal workspace for coding agents; Apache-2.0). No apt
# package and no mise/nix surface here, so install_herdr.sh fetches the static
# release binary into ~/opt/bin and verifies it against the SHA-256 herdr
# publishes in herdr.dev/latest.json. Tracks the latest release so fleet update
# keeps every host on the same version; pin with HERDR_VERSION=x.y.z. The agent
# integrations are a separate config-phase step (install.tools.herdr-integrations).
if gff_on install.tools.herdr; then
  if [ -f "${BASE_DIR}/opt/scripts/system/install_herdr.sh" ]; then
    echo "Installing herdr..."
    "${BASE_DIR}/opt/scripts/system/install_herdr.sh" || echo "WARNING: herdr install reported problems; continuing."
  fi
else gff_skip_msg install.tools.herdr; fi

# Install the Snowflake CLI (`snow`). Replaces the old .zshrc daily-maintenance
# pip auto-install, which broke on PEP 668 (externally-managed-environment)
# systems. macOS uses the homebrew-core formula; Linux uses pipx so the system
# Python is untouched.
if gff_on install.tools.snowflake; then
  if [ -f "${BASE_DIR}/opt/scripts/system/install_snowflake_cli.sh" ]; then
    echo "Installing snowflake-cli..."
    "${BASE_DIR}/opt/scripts/system/install_snowflake_cli.sh" || echo "WARNING: snowflake-cli install reported problems; continuing."
  fi
else gff_skip_msg install.tools.snowflake; fi

# only setup these scripts when docker is installed and responsive
if gff_on install.tools.docker; then
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
else gff_skip_msg install.tools.docker; fi

# Git shortcuts: the Gerrit-era ~/.gitenv generator (setup_git_alias.sh) is
# retired — the surviving tools live in opt/profiles/.gitools.sh, symlinked by
# the profiles block above and sourced from .bash_aliases. This block only
# migrates old hosts: remove the generated artifacts so stale fgit-* functions
# (and the fgit-login autorun they carried) can't shadow the new ones.
if gff_on install.tools.git-aliases; then
  for _stale in "${HOME}/.gitenv" "${HOME}/.gitenv.nologin"; do
    if [ -f "${_stale}" ]; then
      echo "Removing retired git-alias artifact: ${_stale}"
      rm -f "${_stale}"
    fi
  done
else gff_skip_msg install.tools.git-aliases; fi

# Non-fatal git identity check: flags user.email/GitHub-account mismatches
# (e.g. GitHub's email-privacy push block) at install time instead of at the
# next failed push. Also runnable on demand via `make git-doctor`.
bash "${BASE_DIR}/opt/scripts/git/git_identity_doctor.sh" || true

# install/update goenv
if gff_on install.runtime.goenv; then
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
else gff_skip_msg install.runtime.goenv; fi

# --- Go PATH activation (phase-independent) --------------------------------
# goenv INSTALLATION is a deps-phase step (repo-independent → cached layer), but
# PATH is NOT inherited across Docker RUN layers. So in the `config` layer goenv
# is present on disk while `go` is absent from PATH, and every later Go build —
# the gff bootstrap just below and all sdk/*/build.sh — degrades to
# "WARNING: 'go' not found" and skips. Post-#217 that was invisible: the sdk
# builds also ran in the deps layer, so the image still shipped binaries, just
# ones baked into the cached layer and stamped `Commit: none` (that layer's
# partial COPY carries no .git). This re-activates an ALREADY-INSTALLED goenv;
# it installs nothing and touches no network. It is a strict no-op whenever `go`
# is already on PATH (the real-machine / `--phase all` case), so behavior there
# is unchanged.
ensure_go_on_path() {
  if command -v go >/dev/null 2>&1; then
    return 0
  fi
  [ -d "${HOME}/.goenv" ] && export PATH="${HOME}/.goenv/bin:${PATH}"
  command -v goenv >/dev/null 2>&1 || return 0
  # Same bash-safe init + PATH-clobber guard as the goenv section above; see
  # docs/mbo/specs/shell-portability.md for why `- bash` is passed explicitly.
  __goenv_path_safe="${PATH}"
  eval "$(goenv init - bash)"
  case ":${PATH}:" in
    *":/usr/bin:"*) : ;;                          # system PATH survived
    *) PATH="${PATH}:${__goenv_path_safe}" ;;     # init clobbered PATH; restore
  esac
  export PATH
  unset __goenv_path_safe
}
ensure_go_on_path
# FAIL HARD in a container build. On a real machine (`--phase all`) a missing Go
# is tolerable — the sdk build scripts warn and skip, and the user may simply not
# want Go. Inside the two-phase Docker build it is a BUG: the config layer would
# silently ship whatever binaries the cached deps layer happened to bake in
# (stamped `Commit: none`), which is exactly how #217 regressed unnoticed while
# CI stayed green. Surface it as a build failure instead of a WARNING.
if [ "$INSTALL_PHASE" != "all" ] && ! command -v go >/dev/null 2>&1; then
  echo "ERROR: install.sh --phase ${INSTALL_PHASE}: no 'go' on PATH after goenv activation." >&2
  echo "       The sdk/*/build.sh steps would silently skip and the image would ship" >&2
  echo "       stale binaries. Check the goenv deps layer and ensure_go_on_path()." >&2
  exit 1
fi

# build gff first so every later step can be feature-flag gated (fail-open:
# if the build fails or gff is absent, all steps run — flags only ever skip).
if gff_bootstrap_ok=false; command -v go >/dev/null 2>&1 && [ -f "${BASE_DIR}/sdk/gff/build.sh" ]; then
  bash "${BASE_DIR}/sdk/gff/build.sh" && gff_bootstrap_ok=true || echo "WARNING: gff build failed; all components will run."
fi
if [ "$gff_bootstrap_ok" = "true" ] && [ -x "${HOME}/opt/bin/gff" ]; then
  # set -a exports the plain VAR=v lines gff emits so GFF_* reaches child
  # scripts (install_windows.sh reads them via `env` for the WSLENV handoff).
  set -a
  eval "$(cd "${BASE_DIR}" && "${HOME}/opt/bin/gff" export --shell 2>/dev/null || true)"
  set +a
  # Register this checkout's namespace so cross-repo consumers (gsl render, from
  # ANY cwd) can resolve the flags. Fail-open: a failure only warns.
  (cd "${BASE_DIR}" && "${HOME}/opt/bin/gff" install >/dev/null 2>&1) \
    || echo "WARNING: gff install (namespace registration) failed; gsl link flags fail open (links stay on)."
fi
# Re-assert build-phase overrides: the gff export above can have overwritten the
# GFF_* the later runtime gates (pyenv/rbenv/nvm) read.
apply_install_phase

# install/update pyenv
if gff_on install.runtime.pyenv; then
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
else gff_skip_msg install.runtime.pyenv; fi

# install/update rbenv
if gff_on install.runtime.rbenv; then
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
else gff_skip_msg install.runtime.rbenv; fi

# install nvm
if gff_on install.runtime.nvm; then
  if [ ! -d "${HOME}/.nvm" ]; then
    echo "Installing nvm..."
    curl -fsSL -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash > /dev/null 2>&1
  fi
else gff_skip_msg install.runtime.nvm; fi

# install Antigravity CLI (agy) — Gemini CLI's successor (Gemini CLI EOL 2026-06-18)
if gff_on install.ai.antigravity; then
  if [ -f "${BASE_DIR}/opt/scripts/system/antigravity_install.sh" ]; then
    echo "Installing Antigravity CLI..."
    "${BASE_DIR}/opt/scripts/system/antigravity_install.sh"
  fi

  # Retired Gemini CLI leftovers: consent-based teardown (prompts when leftovers
  # are found; --keep marker suppresses the ask forever; no-op in CI/non-TTY).
  if [ -f "${BASE_DIR}/opt/scripts/system/gemini_teardown.sh" ]; then
    "${BASE_DIR}/opt/scripts/system/gemini_teardown.sh" || echo "WARNING: gemini teardown reported problems; continuing."
  fi
else gff_skip_msg install.ai.antigravity; fi

# Google CLI (Antigravity & Workspace) Setup
if gff_on install.ai.google-cli; then
  if [ -f "${BASE_DIR}/opt/scripts/system/google-cli-setup.sh" ]; then
    # This configures gws for BOTH Antigravity CLI and Claude Code via shared skills.
    echo "Setting up Google CLI (Antigravity & Workspace)..."
    "${BASE_DIR}/opt/scripts/system/google-cli-setup.sh"
  fi
else gff_skip_msg install.ai.google-cli; fi

# Antigravity hooks are provisioned by install_antigravity_skills.sh (called
# above): guard scripts are copied into ~/.gemini/config/hooks/ and the wiring
# is rendered to ~/.gemini/config/hooks.json. agy owns its own settings file
# (~/.gemini/antigravity-cli/settings.json) — nothing to merge or symlink here.

# install Claude Code CLI (macOS via brew cask, Linux/WSL via npm)
if gff_on install.ai.claude; then
  if [ -f "${BASE_DIR}/opt/scripts/system/claude_install.sh" ]; then
    echo "Installing Claude Code CLI..."
    "${BASE_DIR}/opt/scripts/system/claude_install.sh"
  fi
else gff_skip_msg install.ai.claude; fi

# Claude settings + hooks are provisioned by install_claude_skills.sh (called
# above): ~/.claude/settings.json is a host-owned file with the forced subset
# (hooks, statusLine, deny/ask) merged in, and hooks are copied into
# ~/.claude/hooks/. No symlink and no repo-internal host copy here.

# Sync AI plugins from the manifest (ai/plugins.yaml). Ensure-only: installs +
# enables the listed plugins; never removes anything. Runs after the Claude CLI
# (claude_install.sh) and yq are installed.
if gff_on install.ai.plugins; then
  if [ -f "${BASE_DIR}/opt/scripts/system/sync-plugins.sh" ]; then
    echo "Syncing AI plugins..."
    "${BASE_DIR}/opt/scripts/system/sync-plugins.sh" || echo "WARNING: plugin sync reported problems; continuing."
  fi
else gff_skip_msg install.ai.plugins; fi

# Install AI teams: transform ai/teams personas into native agents for Claude,
# Antigravity, and Ollama. Runs after yq + the assistant configs. Validates the source
# first; each tool emit degrades gracefully, so a teams problem never aborts bootstrap.
if gff_on install.ai.teams; then
  if [ -f "${BASE_DIR}/opt/scripts/system/install_ai_teams.sh" ]; then
    echo "Installing AI teams..."
    "${BASE_DIR}/opt/scripts/system/install_ai_teams.sh" || echo "WARNING: AI teams install reported problems; continuing."
  fi
else gff_skip_msg install.ai.teams; fi

# WSL only, OPT-IN (fail-closed): build wlink and pin
#
# Placed with the other sdk builds ON PURPOSE: it needs `go`, which is not on
# PATH until the goenv install and ensure_go_on_path above. Higher up, build.sh
# printed "go not found", exited 0, and wlink was silently never installed. the resolver that knows
# your fleet. From WSL, `ssh <fleet-host>` stalls ~20s and dies with "Temporary
# failure in name resolution" while `ssh <ip>` works, because WSL2 points
# resolv.conf at the Windows NAT DNS proxy, which answers from whatever resolver
# Windows treats as primary -- normally the ISP's, which has never heard of the
# fleet. wlink probes EVERY per-interface resolver Windows knows (the right one
# is frequently on a VPN interface, NOT the default route) and pins the one that
# answers, reversibly.
#
# gff_opt_in, NOT gff_on: this rewrites host DNS, so an unset flag, a missing
# gff binary, or a machine where the export never happened must all mean DO NOT
# BUILD. That is a deliberate departure from the other install.sdk.* flags.
#   gff set install.sdk.wlink true
# Undo on any machine:  wlink unpin
if gff_opt_in install.sdk.wlink; then
  if [ -f "${BASE_DIR}/sdk/wlink/build.sh" ]; then
    echo "Installing wlink (WSL link: tunnel + resolver)..."
    bash "${BASE_DIR}/sdk/wlink/build.sh" || echo "WARNING: wlink build reported problems; continuing."
    # Pinning is best-effort by design: it declines safely (exit 0, no write)
    # when the tunnel is down, so install.sh never fails over a link that
    # happens to be unavailable right now.
    # The pin writes under /etc, so it needs root. install.sh cached sudo
    # credentials up front, so this does not prompt again mid-run.
    if [ -x "${HOME}/opt/bin/wlink" ]; then
      if [ "$(id -u)" -eq 0 ]; then
        "${HOME}/opt/bin/wlink" pin || echo "WARNING: wlink pin reported problems; continuing."
      elif command -v sudo >/dev/null 2>&1; then
        sudo -E "${HOME}/opt/bin/wlink" pin || echo "WARNING: wlink pin reported problems; continuing."
      else
        echo "WARNING: wlink installed but cannot pin without root; run: sudo wlink pin"
      fi
    fi
  fi
else
  echo "SKIP (gff: install.sdk.wlink is opt-in and not enabled)"
fi

# build and install gss
if gff_on install.sdk.gss; then
  if [ -f "${BASE_DIR}/sdk/gss/build.sh" ]; then
    echo "Installing gss (dotfiles manager)..."
    bash "${BASE_DIR}/sdk/gss/build.sh"
    if [ -f "${HOME}/opt/bin/gss" ]; then
        echo "--------------------------------------------------"
        "${HOME}/opt/bin/gss" version
        echo "--------------------------------------------------"
    fi
  fi
else gff_skip_msg install.sdk.gss; fi

# build and install tmux-mg
if gff_on install.sdk.tmux-mgr; then
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
else gff_skip_msg install.sdk.tmux-mgr; fi

# build and install wol
if gff_on install.sdk.wol; then
  if [ -f "${BASE_DIR}/sdk/wol/build.sh" ]; then
    echo "Installing wol (Wake-on-LAN utility)..."
    bash "${BASE_DIR}/sdk/wol/build.sh"
    if [ -f "${HOME}/opt/bin/wol" ]; then
        echo "--------------------------------------------------"
        "${HOME}/opt/bin/wol" version
        echo "--------------------------------------------------"
    fi
  fi
else gff_skip_msg install.sdk.wol; fi

# build and install fleet
if gff_on install.sdk.fleet; then
  if [ -f "${BASE_DIR}/sdk/fleet/build.sh" ]; then
    echo "Installing fleet (dotfiles install-status checker)..."
    bash "${BASE_DIR}/sdk/fleet/build.sh"
    if [ -f "${HOME}/opt/bin/fleet" ]; then
        echo "--------------------------------------------------"
        "${HOME}/opt/bin/fleet" version
        echo "--------------------------------------------------"
    fi
  fi
else gff_skip_msg install.sdk.fleet; fi

# build and install gsl
if gff_on install.sdk.gsl; then
  if [ -f "${BASE_DIR}/sdk/gsl/build.sh" ]; then
    echo "Installing gsl (Go status line)..."
    # This script has NO `set -e`, so an unchecked build.sh failure is silently
    # SWALLOWED: install.sh would go on to print "Installation complete!" and exit
    # 0 while gsl had not built at all. build.sh ends with the dependency + seam
    # gate (sdk/gsl/scripts/check-deps.sh), so swallowing its exit status defeats
    # the gate entirely — the exact hole this closes.
    if ! bash "${BASE_DIR}/sdk/gsl/build.sh"; then
        echo "ERROR: gsl build failed (see the build/seam-gate output above)." >&2
        exit 1
    fi
    if [ -f "${HOME}/opt/bin/gsl" ]; then
        echo "--------------------------------------------------"
        "${HOME}/opt/bin/gsl" version
        echo "--------------------------------------------------"
    fi
  fi
else gff_skip_msg install.sdk.gsl; fi

# build and install gff (git fast features). This is the LATER duplicate of the
# bootstrap build above — it exists so the version banner matches the other sdk
# tools and so the rebuild can be flag-skipped. Only THIS duplicate is gated;
# the bootstrap build itself is never gated (it is what makes gating possible).
if gff_on install.sdk.gff; then
  if [ -f "${BASE_DIR}/sdk/gff/build.sh" ]; then
    echo "Installing gff (git fast features)..."
    bash "${BASE_DIR}/sdk/gff/build.sh"
    if [ -f "${HOME}/opt/bin/gff" ]; then
        echo "--------------------------------------------------"
        "${HOME}/opt/bin/gff" version
        echo "--------------------------------------------------"
    fi
  fi
else gff_skip_msg install.sdk.gff; fi

# Configure the Nerd Font (MesloLGS Nerd Font) used by gsl's powerline style.
# Runs AFTER the gsl build so both the gsl skill files (linked by sync-skills
# above) and the freshly-built ~/opt/bin/gsl exist. OS-dispatch to the
# gsl-packaged installers under sdk/gsl/scripts/.
if gff_on install.fonts.nerd-font; then
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
else gff_skip_msg install.fonts.nerd-font; fi

# install fnm
if gff_on install.runtime.fnm; then
  if ! command -v fnm &> /dev/null; then
    echo "Installing fnm..."
    curl -fsSL https://fnm.vercel.app/install | bash -s -- --skip-shell > /dev/null 2>&1
  fi
else gff_skip_msg install.runtime.fnm; fi

# setup sshd server if requested
if gff_on install.network.sshd; then
  if [ -f "${HOME}/.sshd.env" ]; then
    "${BASE_DIR}/opt/scripts/network/sshd_run.sh"
  fi
else gff_skip_msg install.network.sshd; fi

if gff_on install.system.gitrepos; then
  if [ -f "${HOME}/.gitrepos" ] ; then
    cd "${HOME}" || exit 1
    "${HOME}/.gitrepos"
  fi
else gff_skip_msg install.system.gitrepos; fi

# Keep the oh-my-zsh clone current. .gitrepos above clones it but is told
# never to pull (";false" in .repos.env), so upstream plugin fixes never
# landed. Must run AFTER the gitrepos block so a fresh clone exists.
# Fast-forward only; warns and continues on a diverged/offline clone.
if gff_on install.shell.oh-my-zsh-update; then
  bash "${BASE_DIR}/opt/scripts/system/oh-my-zsh_update.sh" ||
    echo "WARNING: oh-my-zsh update reported problems; continuing."
else gff_skip_msg install.shell.oh-my-zsh-update; fi

# Load Nano Platform environment
if gff_on install.system.nano-profile; then
for shell_config in "$HOME/.zshrc" "$HOME/.profile"; do
    if [ -f "$shell_config" ]; then
        if ! grep -q "\.nano_profile" "$shell_config"; then
            echo "Adding .nano_profile source to $shell_config"
            echo '[ -f "$HOME/.nano_profile" ] && . "$HOME/.nano_profile"' >> "$shell_config"
        fi
    fi
done
else gff_skip_msg install.system.nano-profile; fi

# Windows/WSL only — DEFERRED execution (the y/n/s answer was captured up top):
# runs AFTER the gff bootstrap so install.windows.* overrides are exported —
# including on a fresh system. See docs/mbo/specs/gff-install-flow.md.
if gff_on install.windows.desktop-deploy; then
  if [ -f "${BASE_DIR}/opt/bin/install_windows.sh" ]; then
    bash "${BASE_DIR}/opt/bin/install_windows.sh" "${BASE_DIR}" --deferred
  fi
else gff_skip_msg install.windows.desktop-deploy; fi

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

# --- install stamp (fleet) -------------------------------------------------
# LAST action of a successful run: record the commit that was installed so
# `fleet status` can tell "pulled" from "actually installed". Phase-gated
# inside the script (a Docker deps/config layer must never stamp), and it
# never fails the install.
if [ -f "${BASE_DIR}/opt/scripts/system/install-stamp.sh" ]; then
  bash "${BASE_DIR}/opt/scripts/system/install-stamp.sh" "${BASE_DIR}" || true
fi
