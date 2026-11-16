package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/provenance"
)

var provenanceHosts []string

var provenanceCmd = &cobra.Command{
	Use:   "provenance",
	Short: "Provenance trailers for assistant commits (Built-By, Built-On, Built-Session, Hooks)",
}

var provenanceStampCmd = &cobra.Command{
	Use:   "stamp <commit-msg-file>",
	Short: "Add provenance trailers to a commit message (called by the commit-msg hook)",
	Long: `When an assistant is detected (GSS_BUILT_BY, CLAUDECODE=1, or AI_AGENT), adds
the trailers Built-By, Built-On (a hash of the short host name, never the
name), Built-Session (when known) and Hooks: global to the message file with
git interpret-trailers. A trailer whose key is already there is left alone,
so an amend does not stamp twice. With no assistant detected it does nothing.

The commit-msg privacy hook calls it after its check. It always exits 0, so
it cannot fail a commit.`,
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := stampMessage(context.Background(), git.NewSystemRunner(), args, provenance.Detect(os.Getenv, hostname())); err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), err)
		}
	},
}

var provenanceShowCmd = &cobra.Command{
	Use:   "show [--host <name>...]",
	Short: "Print this machine's provenance, or the Built-On id of named hosts",
	Long: `With no flags, prints what gss would stamp here: the harness, session and
this host's Built-On id. With --host (repeatable), prints the Built-On id of
each name, so an escape hunt can map an id in a trailer back to a fleet host.`,
	Run: func(cmd *cobra.Command, args []string) {
		showProvenance(cmd.OutOrStdout(), provenance.Detect(os.Getenv, hostname()), provenanceHosts)
	},
}

func stampMessage(ctx context.Context, r git.Runner, args []string, info provenance.Info) error {
	if len(args) != 1 {
		return fmt.Errorf("provenance stamp: want <commit-msg-file>, got %d args", len(args))
	}
	trailers := provenance.Trailers(info)
	if len(trailers) == 0 {
		return nil
	}
	argv := []string{"interpret-trailers", "--in-place", "--if-exists", "doNothing"}
	for _, t := range trailers {
		argv = append(argv, "--trailer", t)
	}
	argv = append(argv, args[0])
	if out, err := r.Run(ctx, argv[0], argv[1:]...); err != nil {
		return fmt.Errorf("provenance stamp: %w: %s", err, out)
	}
	return nil
}

func showProvenance(w io.Writer, info provenance.Info, hosts []string) {
	if len(hosts) > 0 {
		for _, h := range hosts {
			fmt.Fprintf(w, "%s\t%s\n", provenance.HostID(h), h)
		}
		return
	}
	harness := info.Harness
	if harness == "" {
		harness = "(no assistant detected: nothing is stamped)"
	}
	fmt.Fprintf(w, "built-by\t%s\nbuilt-on\t%s\n", harness, info.HostID)
	if info.Session != "" {
		fmt.Fprintf(w, "session\t%s\n", info.Session)
	}
}

func hostname() string {
	h, _ := os.Hostname()
	return h
}

func init() {
	provenanceShowCmd.Flags().StringArrayVar(&provenanceHosts, "host", nil, "Print the Built-On id of this host name (repeatable)")
	provenanceCmd.AddCommand(provenanceStampCmd, provenanceShowCmd)
	rootCmd.AddCommand(provenanceCmd)
}

// provenanceHere is this process's provenance, for the PR body section.
func provenanceHere() *provenance.Info {
	info := provenance.Detect(os.Getenv, hostname())
	return &info
}
