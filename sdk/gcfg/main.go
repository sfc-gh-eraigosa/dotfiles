// gcfg — GitHub settings as code. The module root stays `go run`-able:
// `go run github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg@<tag> <verb> …`.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/cmd"
)

// exitCode maps an error to the process exit code (plan §3.5):
// 0 ok/clean · 1 drift or apply left findings · 2 usage, no credential,
// unreadable family, non-TTY apply without --yes.
func exitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, cmd.ErrUsage):
		return 2
	case errors.Is(err, cmd.ErrFindings):
		return 1
	default:
		return 1
	}
}

// run executes the CLI with args and returns the exit code. A findings
// error is silent: the verb already printed its report.
func run(args []string, stderr io.Writer) int {
	root := cmd.NewRootCmd()
	root.SetArgs(args)
	err := root.Execute()
	if err != nil && !errors.Is(err, cmd.ErrFindings) {
		fmt.Fprintf(stderr, "gcfg: %v\n", err)
	}
	return exitCode(err)
}

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}
