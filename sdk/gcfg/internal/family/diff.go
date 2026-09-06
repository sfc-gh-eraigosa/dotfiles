package family

import "github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/schema"

// Differ accumulates the findings and changes one family produces, so every
// family reports the same way: a declared key that disagrees with live is
// drift plus a change, and an undeclared key is left alone.
type Differ struct {
	family   string
	findings []Finding
	changes  []Change
}

// NewDiffer starts a diff for one family.
func NewDiffer(family string) *Differ { return &Differ{family: family} }

// Scalar compares one declared value against live. declared=false means the
// key is not in the file, which is never drift and never a change.
func (d *Differ) Scalar(key string, declared bool, want, live any) {
	if !declared || want == live {
		return
	}
	d.findings = append(d.findings, Finding{Family: d.family, Key: key, Kind: Drift, Want: want, Live: live})
	d.changes = append(d.changes, Change{Family: d.family, Key: key, Op: OpUpdate, Want: want, Live: live, PreImage: live})
}

// List compares a declared string list against live, order-insensitively.
func (d *Differ) List(key string, declared bool, want, live []string) {
	if !declared || SameStrings(want, live) {
		return
	}
	d.findings = append(d.findings, Finding{Family: d.family, Key: key, Kind: Drift, Want: want, Live: live})
	d.changes = append(d.changes, Change{Family: d.family, Key: key, Op: OpUpdate, Want: want, Live: live, PreImage: live})
}

// Report adds a finding that is not a change — an unmanaged extra, or a
// setting GitHub accepted but did not honour.
func (d *Differ) Report(f Finding) {
	f.Family = d.family
	d.findings = append(d.findings, f)
}

// Add records a change with its own operation (create/delete for the list
// families).
func (d *Differ) Add(c Change) {
	c.Family = d.family
	d.changes = append(d.changes, c)
}

// Result returns what the family found and what it would change.
func (d *Differ) Result() ([]Finding, []Change) { return d.findings, d.changes }

// Managed is true when ownership makes undeclared live state gcfg's problem.
func Managed(own schema.Ownership) bool { return own == schema.Full }
