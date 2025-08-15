# export GOPATH=$HOME/GitHub/go
# export PATH=$PATH:$GOPATH/bin
# unset GOROOT
# https://golang.org/doc/gopath_code.html#GOPATH
# export PATH=$PATH:$(go env GOPATH)/bin
if [[ -d $HOME/.goenv ]]; then
    export PATH=$HOME/.goenv/bin:$PATH
fi

# export GOENV_ROOT=$HOME/go
export GOENV_ROOT="$HOME/.goenv"
if [[ "$(uname -r | awk -F'-' '{print $3}')" = "Microsoft" ]] ; then
    export GOENV_ROOT=/mnt/c/Program\ Files/Go
    alias go='go.exe'
fi
export PATH="$GOENV_ROOT/bin:$PATH"

# requires brew install goenv
goenv install latest --skip-existing
eval "$(goenv init -)"

goenv shell $(goenv versions --bare|tail -1)
go version
which go


