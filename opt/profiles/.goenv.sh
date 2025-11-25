# https://golang.org/doc/gopath_code.html#GOPATH

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
        command -v goenv >/dev/null 2>&1 && goenv install latest --skip-existing
    fi
fi
eval "$(goenv init -)"

# set GOENV_VERSION
goenv shell $(goenv versions --bare|tail -1)

# some debug output and default settings to put go in the path
export GO_BINARY=$(goenv which go)
export GO_BINPATH=$(dirname ${GO_BINARY})
export PATH=${GO_BINPATH}:${PATH}

if [[ "$EDITOR_TERMINAL" == "true" ]]; then
    return
fi

export GOTOOLCHAIN="go${GOENV_VERSION}"

echo "GOENV_VERSION => ${GOENV_VERSION}"
echo "GOTOOLCHAIN   => ${GOTOOLCHAIN}"
echo "GO_BINARY     => ${GO_BINARY}"
echo "GO_BINPATH    => ${GO_BINPATH}"
echo "GOPATH        => ${GOPATH}"


# install some go command line tools (once per day)
if __dotfiles_should_run_daily; then
    __dotfiles_touch_daily
    go install github.com/bazelbuild/buildtools/buildifier@latest
    go install golang.org/x/tools/gopls@latest
    go install github.com/go-delve/delve/cmd/dlv@latest
fi

# verify we have the tools (quiet outside of daily run)
if __dotfiles_should_run_daily; then
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

if command -v go >/dev/null 2>&1; then
    echo "✓ $(go version) is installed"
else
    echo "✗ go is not installed properly"
fi

export GOBIN=$(go env -json | jq -r '.GOROOT')/bin

go version
which go


