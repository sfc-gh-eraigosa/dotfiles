# https://golang.org/doc/gopath_code.html#GOPATH
if [[ -d $HOME/.goenv ]]; then
    export PATH=$HOME/.goenv/bin:$PATH
fi

export GOENV_ROOT=$HOME/go
if [[ "$(uname -r | awk -F'-' '{print $3}')" = "Microsoft" ]] ; then
    export GOENV_ROOT=/mnt/c/Program\ Files/Go
    alias go='go.exe'
fi

# requires brew install goenv
goenv install latest --skip-existing
eval "$(goenv init -)"

# set GOENV_VERSION
goenv shell $(goenv versions --bare|tail -1)

# some debug output and default settings to put go in the path
export GO_BINARY=$(goenv which go)
export GO_BINPATH=$(dirname ${GO_BINARY})
export PATH=${GO_BINPATH}:${PATH}
echo "GOENV_VERSION => ${GOENV_VERSION}"
echo "GO_BINARY => ${GO_BINARY}"
echo "GO_BINPATH => ${GO_BINPATH}"

# install some go command line tools
go install github.com/bazelbuild/buildtools/buildifier@latest
go install golang.org/x/tools/gopls@latest
go install github.com/go-delve/delve/cmd/dlv@latest

# verify we have the tools
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

if command -v go >/dev/null 2>&1; then
    echo "✓ $(go version) is installed"
else
    echo "✗ go is not installed properly"
fi

go version
which go


