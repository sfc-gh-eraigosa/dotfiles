# shellcheck shell=bash
# goenv configuration
export GOENV_ROOT="$HOME/go"
export GOENV_PATH_ORDER=front

# Detect if we're in an automated/editor environment
if [[ "$TERM_PROGRAM" == "vscode" ]] || [[ "$TERM_PROGRAM" == "cursor" ]] || [[ -n "$VSCODE_PID" ]] || [[ -n "$CURSOR_PID" ]] || [[ -n "$GEMINI_CLI" ]]; then
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

# Initialize goenv
# PATH-clobber guard (macOS): a misconfigured `goenv init -` can emit a PATH
# assignment that drops the system bin dirs (/usr/bin, /bin, …), breaking every
# coreutil in the login shell. Capture a known-good PATH and restore it if the
# eval strips the system dirs. Shell-agnostic (bash + zsh). See the
# shell-portability standard referenced in CLAUDE.md.
__goenv_path_safe="$PATH"
eval "$(goenv init -)"
case ":${PATH}:" in
    *":/usr/bin:"*) : ;;                          # system PATH survived
    *) PATH="${PATH}:${__goenv_path_safe}" ;;     # init clobbered PATH; restore
esac
export PATH
unset __goenv_path_safe

# Set shell version to latest installed version if not already set by .go-version or local
if [ -z "$GOENV_VERSION" ]; then
    LATEST_GO_VERSION=$(goenv versions --bare 2>/dev/null | tail -1)
    if [ -n "$LATEST_GO_VERSION" ]; then
        # Use 'goenv shell' which handles GOENV_VERSION correctly
        # This requires the shell integration from 'goenv init'
        goenv shell "$LATEST_GO_VERSION"
    fi
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
