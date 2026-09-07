package updexec

import (
	"regexp"
	"strings"
)

// benignPatterns are the stderr lines every healthy run produces: ssh's
// known-hosts and tty notices, git's progress reporting (git writes ALL of it
// to stderr), and sudo's prompt echo. Counting these as warnings would put a
// ⚠ on every host on every run, which is the same as having no warning signal
// at all.
//
// Each pattern is ANCHORED and matches the whole SHAPE of the line, not just a
// prefix. That distinction is load-bearing: git reports its failures through
// the same "remote: " channel as its progress, so a prefix test would have
// silently whitelisted "remote: fatal: repository not found". Listing the
// exact progress verbs keeps this a denylist of known-good shapes rather than
// a guess about what an error looks like — the same reason the design rejected
// classifying stderr by text in the first place.
var benignPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^Warning: Permanently added .* to the list of known hosts\.$`),
	regexp.MustCompile(`^Pseudo-terminal will not be allocated`),
	regexp.MustCompile(`^remote: (Enumerating|Counting|Compressing|Total|Finding|Resolving) `),
	regexp.MustCompile(`^(Receiving|Resolving|Counting|Compressing|Unpacking|Enumerating) (objects|deltas):`),
	regexp.MustCompile(`^From (https?://|git@|/)`),
	// NOTE: no leading-space patterns — Benign trims the line first, so
	// `^ \* branch …` could never match, and every real `git fetch` would put
	// a spurious ⚠ on the row.
	regexp.MustCompile(`^\* \[?new (branch|tag)\]?`),
	regexp.MustCompile(`^\* branch\s+\S+\s+-> \S+$`),
	regexp.MustCompile(`^[0-9a-f]{7,40}\.\.[0-9a-f]{7,40}\s+\S+\s+-> \S+$`),
	regexp.MustCompile(`^\[sudo\] password for `),
}

// Benign reports whether a stderr line is routine chatter rather than a
// warning. It NEVER hides the line — the error pane shows every stderr line;
// this only decides whether the host's row gets a ⚠.
//
// Matching is deliberately conservative: an unknown line is a WARNING. A false
// ⚠ costs a glance; a missed one costs a half-installed host reporting success.
func Benign(line string) bool {
	s := strings.TrimSpace(line)
	if s == "" {
		return true
	}
	for _, re := range benignPatterns {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}
