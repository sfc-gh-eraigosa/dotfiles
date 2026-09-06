// ghapp — GitHub App credential toolkit. The module root stays `go run`-able:
// `go run github.com/sfc-gh-eraigosa/dotfiles/sdk/ghapp@<tag> <verb> …`.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/ghapp/cmd"
)

// exitCode maps an error to the process exit code (plan §3.5):
// 0 ok · 1 error · 2 usage / no credential.
func exitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, cmd.ErrUsage):
		return 2
	default:
		return 1
	}
}

// run executes the CLI with args and returns the exit code; errors are
// printed to stderr with the "ghapp: " prefix. Kept apart from main so the
// mapping is testable without os.Exit.
func run(args []string, stderr io.Writer) int {
	root := cmd.NewRootCmd()
	root.SetArgs(args)
	err := root.Execute()
	if err != nil {
		fmt.Fprintf(stderr, "ghapp: %v\n", err)
	}
	return exitCode(err)
}

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}
