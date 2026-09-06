package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// Injected at build time via -ldflags (see build.sh). Names must stay
// exported and match the -X paths in build.sh or the injection silently
// no-ops and every binary reports "dev".
var (
	Version   = "dev"
	Commit    = "none"
	Dirty     = "false"
	BuildDate = "unknown"
	// Repo is the origin remote the binary was built from, injected verbatim
	// from `git remote get-url origin` — parsing its spelling is commitURL's
	// job, not build.sh's, so the rule is unit-tested Go rather than sed. The
	// default keeps an un-injected `go build` linking to the right place.
	Repo = "https://github.com/sfc-gh-eraigosa/dotfiles"
)

func versionString() string { return fmt.Sprintf("fleet %s (%s)", Version, Commit) }

// bannerVersion is versionString with the commit turned into a clickable
// hyperlink to its page on the forge. The URL lives in an OSC 8 escape, which
// occupies ZERO cells — so the banner's width budget is unchanged and the
// panel border still lines up (lipgloss.Width skips OSC sequences).
//
// A build with no real SHA renders exactly what it always did: a link to
// /commit/none would be a 404 that looks authoritative.
func bannerVersion() string {
	u := commitURL(Repo, Commit)
	if u == "" {
		return versionString()
	}
	return fmt.Sprintf("fleet %s (%s)", Version, osc8(u, Commit))
}

// osc8 wraps text in a hyperlink. ST (ESC \) terminates the sequence rather
// than the legacy BEL, and the closing empty-URL OSC is not optional: without
// it every cell painted after this one stays clickable.
func osc8(url, text string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

// commitURL is the commit's web page, or "" when it cannot be known for sure.
//
// GitHub only, on purpose. Other forges spell the path differently (GitLab
// wants /-/commit/), so a guessed URL would hand the operator a broken link
// that looks exactly like a working one — strictly worse than the plain text
// it replaced. Anything unrecognised falls back to no link at all.
func commitURL(remote, commit string) string {
	base := githubWebURL(remote)
	if base == "" || !isShortSHA(commit) {
		return ""
	}
	return base + "/commit/" + commit
}

// githubWebURL normalises the three remote spellings git hands out — scp-like
// (git@github.com:owner/repo.git), ssh:// and https:// — down to the browser
// URL. Anything that is not github.com, or that does not name both an owner
// and a repo, returns "".
func githubWebURL(remote string) string {
	r := strings.TrimSpace(remote)
	r = strings.TrimSuffix(r, "/")
	r = strings.TrimSuffix(r, ".git")

	const host = "github.com"
	var path string
	switch {
	case strings.HasPrefix(r, "git@"+host+":"):
		path = strings.TrimPrefix(r, "git@"+host+":")
	case strings.HasPrefix(r, "ssh://git@"+host+"/"):
		path = strings.TrimPrefix(r, "ssh://git@"+host+"/")
	case strings.HasPrefix(r, "https://"+host+"/"):
		path = strings.TrimPrefix(r, "https://"+host+"/")
	case strings.HasPrefix(r, "http://"+host+"/"):
		path = strings.TrimPrefix(r, "http://"+host+"/")
	default:
		return ""
	}
	// owner/repo exactly — a bare owner has no commits, and a deeper path is
	// something other than a repository root.
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return "https://" + host + "/" + parts[0] + "/" + parts[1]
}

// isShortSHA reports whether commit is a git object name we can link to. The
// ldflags default ("none") and a hand-built "dev" must both fail this, as must
// anything carrying a suffix — `abc1234-dirty` names no commit.
func isShortSHA(commit string) bool {
	if len(commit) < 7 || len(commit) > 40 {
		return false
	}
	for _, r := range commit {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Run: func(cmd *cobra.Command, _ []string) {
		out := cmd.OutOrStdout()
		fmt.Fprintln(out, versionString())
		fmt.Fprintf(out, "  Dirty:      %s\n", Dirty)
		fmt.Fprintf(out, "  Build Date: %s\n", BuildDate)
		if u := commitURL(Repo, Commit); u != "" {
			fmt.Fprintf(out, "  Commit URL: %s\n", u)
		}
	},
}

func init() { rootCmd.AddCommand(versionCmd) }
