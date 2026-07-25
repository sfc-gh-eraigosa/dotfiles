// gff — git fast features. The module root stays `go run`-able (F11):
// `go run github.com/sfc-gh-eraigosa/dotfiles/sdk/gff@<tag> <verb> …`.
package main

import (
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
