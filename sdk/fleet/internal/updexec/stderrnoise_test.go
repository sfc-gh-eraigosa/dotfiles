package updexec

import "testing"

func TestBenignStderrTable(t *testing.T) {
	benign := []string{
		`Warning: Permanently added 'host-pi' (ED25519) to the list of known hosts.`,
		`Pseudo-terminal will not be allocated because stdin is not a terminal.`,
		`remote: Enumerating objects: 41, done.`,
		`remote: Counting objects: 100% (41/41), done.`,
		`remote: Compressing objects: 100% (12/12), done.`,
		`remote: Total 41 (delta 12), reused 0 (delta 0)`,
		`Receiving objects:  73% (30/41)`,
		`Resolving deltas: 100% (12/12), done.`,
		`Counting objects: 41, done.`,
		`Unpacking objects: 100% (5/5), done.`,
		`From https://github.com/example/dotfiles`,
		`From git@github.com:example/dotfiles`,
		` * branch            main       -> FETCH_HEAD`,
		` * [new branch]      main       -> origin/main`,
		`   72392c9..9484943  main       -> origin/main`,
		`[sudo] password for someone: `,
		``,
		`   `,
	}
	for _, l := range benign {
		if !Benign(l) {
			t.Errorf("must be benign: %q", l)
		}
	}

	real := []string{
		`WARNING: apt-get update failed; installs may be incomplete.`,
		`WARNING: grouped install failed; retrying packages individually...`,
		`sudo: a password is required`,
		`fatal: could not read Username for 'https://github.com'`,
		`E: Unable to locate package foo`,
		`ssh: connect to host host-pi port 22: No route to host`,
		// A benign PREFIX must not whitelist whatever follows it: git reports
		// its failures through the same "remote: " channel as its progress.
		`remote: fatal: repository not found`,
		`remote: Permission to example/dotfiles.git denied`,
	}
	for _, l := range real {
		if Benign(l) {
			t.Errorf("must NOT be benign: %q", l)
		}
	}
}

// TestBenignAcceptsAnScpStyleFetchHeader pins a shape found in a REAL
// capture, not in review: `git fetch` against an scp-style remote
// (github.com:owner/repo — what an ssh clone actually has) prints
// "From github.com:owner/repo" to stderr. The original pattern required
// https://, git@ or a leading /, so it matched neither that nor anything
// else this fleet produces — every healthy update scored a spurious ⚠, which
// is precisely the "warning signal worth nothing" outcome the patterns exist
// to prevent.
func TestBenignAcceptsAnScpStyleFetchHeader(t *testing.T) {
	for _, line := range []string{
		"From github.com:sfc-gh-eraigosa/dotfiles",
		"From git@github.com:sfc-gh-eraigosa/dotfiles",
		"From https://github.com/sfc-gh-eraigosa/dotfiles",
		"From /srv/mirrors/dotfiles.git",
	} {
		if !Benign(line) {
			t.Errorf("Benign(%q) = false, want true — this is a routine fetch header", line)
		}
	}
}

// TestBenignStillRejectsAnErrorWearingTheSameShape guards the loosening
// above. git reports failures through the same channels as progress, so a
// pattern relaxed to accept "From <anything>" must not start accepting error
// text that merely begins with a word and a colon.
func TestBenignStillRejectsAnErrorWearingTheSameShape(t *testing.T) {
	for _, line := range []string{
		"remote: fatal: repository not found",
		"From github.com:owner/repo but then extra words",
		"fatal: could not read Username",
		"error: failed to push some refs",
	} {
		if Benign(line) {
			t.Errorf("Benign(%q) = true, want false — this is a real failure", line)
		}
	}
}
