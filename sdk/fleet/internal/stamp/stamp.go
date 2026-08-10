// Package stamp parses the install stamp written by
// opt/scripts/system/install-stamp.sh (spec F1). Pure text in, struct out.
package stamp

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Stamp records which dotfiles commit a host actually installed, and when.
type Stamp struct {
	Commit, Branch, Hostname string
	InstalledAt              time.Time
}

const shaLen = 40

// Parse reads key=value stamp text. It is deliberately strict about commit
// length and installed_at: a truncated write (power loss mid-install) must
// surface as an error rather than masquerade as a valid install record.
// Unknown keys are ignored so the writer can add fields without breaking
// older readers.
func Parse(s string) (Stamp, error) {
	kv := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		i := strings.Index(line, "=")
		if i <= 0 {
			continue
		}
		kv[line[:i]] = line[i+1:]
	}

	commit := kv["commit"]
	if len(commit) != shaLen {
		return Stamp{}, fmt.Errorf("stamp: commit must be a %d-character sha, got %d", shaLen, len(commit))
	}
	epoch, err := strconv.ParseInt(kv["installed_at"], 10, 64)
	if err != nil {
		return Stamp{}, fmt.Errorf("stamp: bad installed_at %q: %w", kv["installed_at"], err)
	}
	return Stamp{
		Commit:      commit,
		InstalledAt: time.Unix(epoch, 0).UTC(),
		Branch:      kv["branch"],
		Hostname:    kv["hostname"],
	}, nil
}
