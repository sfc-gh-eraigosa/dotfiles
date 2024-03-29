# export GOPATH=$HOME/go
# export GOPATH=$HOME/GitHub/go
# export PATH=$PATH:$GOPATH/bin
# unset GOROOT
# https://golang.org/doc/gopath_code.html#GOPATH
# export PATH=$PATH:$(go env GOPATH)/bin

export GOENV_ROOT="$HOME/.goenv"
export PATH="$GOENV_ROOT/bin:$PATH"
eval "$(goenv init -)"
export PATH="$GOROOT/bin:$PATH"
export PATH="$PATH:$GOPATH/bin"
