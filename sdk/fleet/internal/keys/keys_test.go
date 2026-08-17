package keys

import "testing"

func TestComputeAddsMissingAndFlagsForeignForRemoval(t *testing.T) {
	local := []string{"ssh-ed25519 AAA me@box", "ssh-ed25519 BBB me@pi"}
	remote := []string{"ssh-ed25519 AAA me@box", "ssh-ed25519 ZZZ ci@runner"}
	d := Compute(local, remote)
	if len(d.ToAdd) != 1 || d.ToAdd[0] != "ssh-ed25519 BBB me@pi" {
		t.Fatalf("ToAdd = %v, want the one missing key", d.ToAdd)
	}
	if len(d.ToRemove) != 1 || d.ToRemove[0] != "ssh-ed25519 ZZZ ci@runner" {
		t.Fatalf("ToRemove = %v, want the foreign key", d.ToRemove)
	}
}

// Regression for defect 2 of the absorbed ssh-key-sync.sh, which rewrote
// authorized_keys wholesale from the workstation's *.pub — silently deleting
// CI keys, other machines and colleagues. Compute REPORTS removals; it must
// never be usable as "replace remote with local".
func TestComputeNeverTreatsEmptyRemoteAsWholesaleRemoval(t *testing.T) {
	d := Compute([]string{"ssh-ed25519 AAA me@box"}, nil)
	if len(d.ToRemove) != 0 {
		t.Fatalf("no remote keys must mean nothing to remove, got %v", d.ToRemove)
	}
	if len(d.ToAdd) != 1 {
		t.Fatalf("ToAdd = %v", d.ToAdd)
	}
}

// The inverse: no LOCAL keys must not be read as "remove everything remote"
// without the caller explicitly seeing each removal.
func TestComputeWithNoLocalKeysReportsEveryRemoteAsRemoval(t *testing.T) {
	remote := []string{"ssh-ed25519 AAA me@box", "ssh-ed25519 ZZZ ci@runner"}
	d := Compute(nil, remote)
	if len(d.ToRemove) != 2 {
		t.Fatalf("every remote key must be REPORTED, got %v", d.ToRemove)
	}
	if len(d.ToAdd) != 0 {
		t.Fatalf("nothing to add, got %v", d.ToAdd)
	}
}

func TestComputeIsNoOpWhenIdentical(t *testing.T) {
	k := []string{"ssh-ed25519 AAA me@box"}
	d := Compute(k, k)
	if len(d.ToAdd) != 0 || len(d.ToRemove) != 0 {
		t.Fatalf("expected a no-op, got %+v", d)
	}
}

// Whitespace and trailing-comment differences must not create phantom churn.
func TestComputeNormalizesWhitespace(t *testing.T) {
	d := Compute(
		[]string{"ssh-ed25519   AAA   me@box"},
		[]string{"ssh-ed25519 AAA me@box"},
	)
	if len(d.ToAdd) != 0 || len(d.ToRemove) != 0 {
		t.Fatalf("whitespace-only difference must be a no-op, got %+v", d)
	}
}

// Blank lines and comments in authorized_keys are not keys.
func TestComputeIgnoresBlanksAndComments(t *testing.T) {
	d := Compute(
		[]string{"ssh-ed25519 AAA me@box"},
		[]string{"", "# a comment", "ssh-ed25519 AAA me@box", "   "},
	)
	if len(d.ToAdd) != 0 || len(d.ToRemove) != 0 {
		t.Fatalf("blanks/comments must be ignored, got %+v", d)
	}
}
