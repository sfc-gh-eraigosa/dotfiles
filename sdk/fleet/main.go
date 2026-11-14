package main

import (
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/cmd"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/agent"
)

func main() {
	// fleet is its own SSH_ASKPASS program (see agent.Load); ssh-add's prompt
	// in argv[1] is ignored.
	if sock := os.Getenv("FLEET_ASKPASS_SOCK"); sock != "" {
		if err := agent.AskpassClient(sock, os.Stdout); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	cmd.Execute()
}
