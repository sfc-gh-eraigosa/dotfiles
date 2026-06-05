package main

import (
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/tmux-mgr/cmd"
)

func main() {
	defer cmd.CloseLog()
	cmd.Execute()
}
