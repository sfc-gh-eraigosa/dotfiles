// Package keys computes what would change to bring a host's authorized_keys
// in line with the managed key set (spec F11-F13).
//
// Compute REPORTS differences; it never applies them and it never expresses
// "replace remote with local". That is deliberate: the shell script this
// package replaces rewrote authorized_keys wholesale from the workstation's
// *.pub files, silently deleting CI keys, other machines and colleagues.
// Removals must reach a human before they reach a host.
package keys

import "strings"

// Diff is the reported delta. ToRemove requires explicit confirmation.
type Diff struct{ ToAdd, ToRemove []string }

// norm collapses whitespace so formatting differences don't create churn.
func norm(s string) string { return strings.Join(strings.Fields(s), " ") }

// isKey filters out the blank lines and comments authorized_keys may contain.
func isKey(s string) bool {
	t := strings.TrimSpace(s)
	return t != "" && !strings.HasPrefix(t, "#")
}

func Compute(local, remote []string) Diff {
	var d Diff
	have := map[string]bool{}
	for _, r := range remote {
		if isKey(r) {
			have[norm(r)] = true
		}
	}
	want := map[string]bool{}
	for _, l := range local {
		if isKey(l) {
			want[norm(l)] = true
		}
	}
	for _, l := range local {
		if isKey(l) && !have[norm(l)] {
			d.ToAdd = append(d.ToAdd, norm(l))
		}
	}
	for _, r := range remote {
		if isKey(r) && !want[norm(r)] {
			d.ToRemove = append(d.ToRemove, norm(r))
		}
	}
	return d
}
