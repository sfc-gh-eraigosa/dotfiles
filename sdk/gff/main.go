// gff — git fast features. The module root stays `go run`-able (F11):
// `go run github.com/sfc-gh-eraigosa/dotfiles/sdk/gff@<tag> <verb> …`.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/cmd"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
)

func main() {
	if err := cmd.Execute(); err != nil {
		switch {
		case errors.Is(err, resolve.ErrUnknownKey) ||
			errors.Is(err, resolve.ErrUnknownOption) ||
			errors.Is(err, resolve.ErrUnknownSource) ||
			errors.Is(err, resolve.ErrWrongFlagType):
			// exit 2: usage/definition errors
			fmt.Fprintf(os.Stderr, "gff: %v\n", err)
			os.Exit(2)
		case cmd.IsExit1Silent(err):
			// exit 1 silent: off / not-selected — no stderr output
			os.Exit(1)
		default:
			fmt.Fprintf(os.Stderr, "gff: %v\n", err)
			os.Exit(1)
		}
	}
}
