package main

import (
	"github.com/eraigosa/dotfiles/src/tmux-mgr/cmd"
)

func main() {
	defer cmd.CloseLog()
	cmd.Execute()
}
