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

// exitCode maps an error to the appropriate exit code and whether to print to stderr.
// Returns (exitCode, silent) where silent=true means don't print to stderr.
func exitCode(err error) (code int, silent bool) {
	if err == nil {
		return 0, false
	}
	switch {
	case errors.Is(err, resolve.ErrUnknownKey) ||
		errors.Is(err, resolve.ErrUnknownOption) ||
		errors.Is(err, resolve.ErrUnknownSource) ||
		errors.Is(err, resolve.ErrWrongFlagType):
		// exit 2: usage/definition errors
		return 2, false
	case cmd.IsExit1Silent(err):
		// exit 1 silent: off / not-selected — no stderr output
		return 1, true
	default:
		// exit 1 with error message
		return 1, false
	}
}

func main() {
	if err := cmd.Execute(); err != nil {
		code, silent := exitCode(err)
		if !silent {
			fmt.Fprintf(os.Stderr, "gff: %v\n", err)
		}
		os.Exit(code)
	}
}
