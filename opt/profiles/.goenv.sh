# https://golang.org/doc/gopath_code.html#GOPATH

export GOENV_PATH_ORDER=front

# Detect if we're in VSCode/Cursor terminal
if [[ "$TERM_PROGRAM" == "vscode" ]] || [[ "$TERM_PROGRAM" == "cursor" ]] || [[ -n "$VSCODE_PID" ]] || [[ -n "$CURSOR_PID" ]]; then
    export EDITOR_TERMINAL=true
else
    export EDITOR_TERMINAL=false
fi

# Daily maintenance cache for expensive goenv/go tool operations
__DOTFILES_DAILY_CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/dotfiles"
__DOTFILES_DAILY_STAMP_FILE="${__DOTFILES_DAILY_CACHE_DIR}/daily_maintenance.stamp"
__dotfiles_should_run_daily() {
    [ -d "${__DOTFILES_DAILY_CACHE_DIR}" ] || mkdir -p "${__DOTFILES_DAILY_CACHE_DIR}"
    [ -f "${__DOTFILES_DAILY_STAMP_FILE}" ] || return 0
    local now mtime
    now=$(date +%s)
    # macOS uses stat -f, Linux uses stat -c
    if mtime=$(stat -f %m "${__DOTFILES_DAILY_STAMP_FILE}" 2>/dev/null); then
        :
    else
        mtime=$(stat -c %Y "${__DOTFILES_DAILY_STAMP_FILE}" 2>/dev/null || echo 0)
    fi
    [ $(( now - mtime )) -ge 86400 ]
}
__dotfiles_touch_daily() {
    : > "${__DOTFILES_DAILY_STAMP_FILE}" 2>/dev/null || touch "${__DOTFILES_DAILY_STAMP_FILE}" 2>/dev/null
}

if [[ -d $HOME/.goenv ]]; then
    export PATH=$HOME/.goenv/bin:$PATH
fi

# Exit early if goenv is not found
if ! command -v goenv &>/dev/null; then
    echo "WARNING: goenv not found. Please run install.sh to install it."
    return 1
fi

export GOENV_ROOT=$HOME/go
if [[ "$(uname -r | awk -F'-' '{print $3}')" = "Microsoft" ]] ; then
    export GOENV_ROOT=/mnt/c/Program\ Files/Go
    alias go='go.exe'
fi

# requires brew install goenv
# Install/ensure latest Go version at most once per day in non-editor terminals
if [[ "$EDITOR_TERMINAL" == "false" ]]; then
    if __dotfiles_should_run_daily; then
        __dotfiles_touch_daily
        # Quietly try to install if nothing exists, or ensure latest is there
        goenv install latest --skip-existing >/dev/null 2>&1 &
    fi
fi
eval "$(goenv init -)"

# set GOENV_VERSION if any version is installed
LATEST_GO_VERSION=$(goenv versions --bare 2>/dev/null | tail -1)
if [ -n "$LATEST_GO_VERSION" ]; then
    goenv shell "$LATEST_GO_VERSION"
    export GOENV_VERSION="$LATEST_GO_VERSION"
fi

# some debug output and default settings to put go in the path
export GO_BINARY=$(goenv which go 2>/dev/null)
if [ -n "$GO_BINARY" ]; then
    export GO_BINPATH=$(dirname "${GO_BINARY}")
    export PATH="${GO_BINPATH}:${PATH}"
fi

if [[ "$EDITOR_TERMINAL" == "true" ]]; then
    export GOTOOLCHAIN="local"
    return
fi

if [ -n "$GOENV_VERSION" ] && [[ "$EDITOR_TERMINAL" == "false" ]]; then
    # echo "GOENV_VERSION => ${GOENV_VERSION}"
    # echo "GOTOOLCHAIN   => ${GOTOOLCHAIN}"
    :
fi

if [ -n "$GO_BINARY" ]; then
    # echo "GO_BINARY     => ${GO_BINARY}"
    # echo "GO_BINPATH    => ${GO_BINPATH}"
    # echo "GOPATH        => ${GOPATH}"

    # install some go command line tools (once per day)
    if __dotfiles_should_run_daily; then
        __dotfiles_touch_daily
        go install github.com/bazelbuild/buildtools/buildifier@latest >/dev/null 2>&1 &
        go install golang.org/x/tools/gopls@latest >/dev/null 2>&1 &
        go install github.com/go-delve/delve/cmd/dlv@latest >/dev/null 2>&1 &
    fi
fi

# verify we have the tools (quiet outside of daily run)
if __dotfiles_should_run_daily && [ -n "$GO_BINARY" ]; then
    # Check if tools are installed and working
    if command -v buildifier >/dev/null 2>&1; then
        echo "✓ buildifier $(buildifier --version) is installed"
    else
        echo "✗ buildifier is not installed properly"
    fi
    if command -v gopls >/dev/null 2>&1; then
        echo "✓ gopls $(gopls version) is installed"
    else
        echo "✗ gopls is not installed properly"
    fi
    if command -v dlv >/dev/null 2>&1; then
        echo "✓ delve $(dlv version) is installed"
    else
        echo "✗ delve debugger is not installed properly"
    fi
fi

if [ -n "$GO_BINARY" ] && command -v go >/dev/null 2>&1; then
    # echo "✓ $(go version) is installed"
    export GOBIN=$(go env -json 2>/dev/null | jq -r '.GOROOT' 2>/dev/null)/bin
    # go version
    # which go
    :
else
    if [[ "$EDITOR_TERMINAL" == "false" ]]; then
      echo "✗ go is not installed properly or not in PATH"
    fi
fi


