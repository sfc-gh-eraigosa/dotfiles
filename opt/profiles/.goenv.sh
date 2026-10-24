# shellcheck shell=bash
# goenv configuration
# ONE goenv tree: $HOME/.goenv, the clone install.sh maintains and installs the
# pinned Go into (.go-version via opt/profiles/versions.tsv). The fleet pins
# probe and opt/etc/fleet/pins.yaml read the same tree. This was $HOME/go, a
# second tree that pin bumps never reached: every host's PINS read "ok" while
# interactive shells kept running the old Go. GOPATH is unaffected (goenv
# keeps it at $HOME/go/<version>). Test: opt/profiles/goenv_profile_test.sh.
export GOENV_ROOT="$HOME/.goenv"
export GOENV_PATH_ORDER=front

# Detect if we're in an automated/editor environment
if [[ "$TERM_PROGRAM" == "vscode" ]] || [[ "$TERM_PROGRAM" == "cursor" ]] || [[ -n "$VSCODE_PID" ]] || [[ -n "$CURSOR_PID" ]] || [[ -n "${GEMINI_CLI:-}" ]] || [[ -n "${ANTIGRAVITY_AGENT:-}" ]]; then
    export EDITOR_TERMINAL=true
else
    export EDITOR_TERMINAL=false
fi

if [[ -d "$HOME/.goenv" ]]; then
    export PATH="$HOME/.goenv/bin:$PATH"
fi

# Exit early if goenv is not found
if ! command -v goenv &>/dev/null; then
    return 0
fi

if [[ "$(uname -r | awk -F'-' '{print $3}')" = "Microsoft" ]] ; then
    export GOENV_ROOT=/mnt/c/Program\ Files/Go
    alias go='go.exe'
fi

# Initialize goenv. Pass the CURRENT shell explicitly: bare `goenv init -`
# infers it from $SHELL, so a bash session sourcing this while $SHELL=zsh would
# eval zsh code (a `read -rA` PATH loop) under bash and error out, emptying PATH.
# Detect the live shell instead; empty falls back to goenv's own detection.
if [ -n "${ZSH_VERSION:-}" ]; then _goenv_shell=zsh
elif [ -n "${BASH_VERSION:-}" ]; then _goenv_shell=bash
else _goenv_shell=""; fi
# PATH-clobber guard (belt-and-suspenders): if any goenv build still drops the
# system bin dirs (/usr/bin, /bin, …), restore a known-good PATH so the login
# shell keeps working. See the shell-portability standard referenced in CLAUDE.md.
__goenv_path_safe="$PATH"
eval "$(goenv init - "${_goenv_shell}")"
case ":${PATH}:" in
    *":/usr/bin:"*) : ;;                          # system PATH survived
    *) PATH="${PATH}:${__goenv_path_safe}" ;;     # init clobbered PATH; restore
esac
export PATH
unset __goenv_path_safe _goenv_shell

# Pick the shell's Go unless one is already chosen (GOENV_VERSION, or a
# .go-version / goenv local that goenv itself resolves). The `goenv global`
# install.sh set is the PIN, so it wins whenever it is installed; only a
# global of "system" or a not-yet-installed version falls back to the newest
# installed Go. (This used to always pick the newest, overriding the pin.)
# A pick made here is exported as GOENV_VERSION, so it is also marked in
# GOENV_VERSION_AUTO: a child shell (a herdr/tmux pane under a server started
# before the pin was installed) re-decides it instead of treating it as explicit.
if [ -n "${GOENV_VERSION:-}" ] && [ "${GOENV_VERSION}" = "${GOENV_VERSION_AUTO:-}" ]; then
    unset GOENV_VERSION GOENV_VERSION_AUTO
fi
if [ -z "${GOENV_VERSION:-}" ]; then
    _goenv_global=$(goenv global 2>/dev/null)
    if [ -z "$_goenv_global" ] || [ "$_goenv_global" = "system" ] || [ ! -d "$GOENV_ROOT/versions/$_goenv_global" ]; then
        LATEST_GO_VERSION=$(goenv versions --bare 2>/dev/null | tail -1)
        if [ -n "$LATEST_GO_VERSION" ]; then
            # Use 'goenv shell' which handles GOENV_VERSION correctly
            # This requires the shell integration from 'goenv init'
            goenv shell "$LATEST_GO_VERSION"
            export GOENV_VERSION_AUTO="$LATEST_GO_VERSION"
        fi
    fi
    unset _goenv_global
fi

# Ensure goenv shims are in PATH
if [[ -d "$GOENV_ROOT/shims" ]]; then
    export PATH="$GOENV_ROOT/shims:$PATH"
fi

# If we are in an editor/prompt environment, set GOTOOLCHAIN to local and exit early
if [[ "$EDITOR_TERMINAL" == "true" ]]; then
    export GOTOOLCHAIN="local"
    return
fi

# Daily maintenance for tools (non-editor terminals only)
__DOTFILES_DAILY_CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/dotfiles"
__DOTFILES_DAILY_STAMP_FILE="${__DOTFILES_DAILY_CACHE_DIR}/daily_maintenance_tools.stamp"
__dotfiles_should_run_daily_tools() {
    [ -d "${__DOTFILES_DAILY_CACHE_DIR}" ] || mkdir -p "${__DOTFILES_DAILY_CACHE_DIR}"
    [ -f "${__DOTFILES_DAILY_STAMP_FILE}" ] || return 0
    local now mtime
    now=$(date +%s)
    if mtime=$(stat -f %m "${__DOTFILES_DAILY_STAMP_FILE}" 2>/dev/null); then
        :
    else
        mtime=$(stat -c %Y "${__DOTFILES_DAILY_STAMP_FILE}" 2>/dev/null || echo 0)
    fi
    [ $(( now - mtime )) -ge 86400 ]
}
__dotfiles_touch_daily_tools() {
    : > "${__DOTFILES_DAILY_STAMP_FILE}" 2>/dev/null || touch "${__DOTFILES_DAILY_STAMP_FILE}" 2>/dev/null
}

if __dotfiles_should_run_daily_tools && command -v go &>/dev/null; then
    (
        go install github.com/bazelbuild/buildtools/buildifier@latest >/dev/null 2>&1
        go install golang.org/x/tools/gopls@latest >/dev/null 2>&1
        go install github.com/go-delve/delve/cmd/dlv@latest >/dev/null 2>&1
        __dotfiles_touch_daily_tools
    ) &
fi
