package cmd

import (
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
)

// recordingRunner captures every remote command so a test can assert on what
// was actually sent over the wire.
type recordingRunner struct {
	fake runner.Fake
	log  *[]string
}

func (r recordingRunner) Run(host string, argv ...string) (string, error) {
	*r.log = append(*r.log, strings.Join(argv, " "))
	return r.fake.Run(host, argv...)
}
func (r recordingRunner) RunInteractive(h string, a ...string) error {
	return r.fake.RunInteractive(h, a...)
}

// The absorbed shell script scp'd the PRIVATE key to every host (defect 1).
// Nothing in the sync path may transfer private key material.
func TestKeysSyncSendsOnlyPublicKeyMaterial(t *testing.T) {
	var sent []string
	r := recordingRunner{fake: runner.Fake{Out: map[string]string{"alpha": ""}}, log: &sent}
	if err := syncKeyToHost(r, "alpha", "ssh-ed25519 AAA me@box"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(sent, " ")
	for _, forbidden := range []string{"BEGIN OPENSSH PRIVATE KEY", "scp ", "id_ed25519 ", "id_rsa "} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("private key material or file transfer detected (%q): %v", forbidden, sent)
		}
	}
	if !strings.Contains(joined, "ssh-ed25519 AAA") {
		t.Fatalf("public key was not sent: %v", sent)
	}
	if !strings.Contains(joined, "authorized_keys") {
		t.Fatalf("key was not appended to authorized_keys: %v", sent)
	}
}

// Defect 4: the shell script swallowed failures with || true and 2>/dev/null.
func TestKeysSyncReportsPerHostFailure(t *testing.T) {
	r := runner.Fake{Err: map[string]error{"dead": runner.ErrFake}}
	err := syncKeyToHost(r, "dead", "ssh-ed25519 AAA me@box")
	if err == nil {
		t.Fatal("a failing host must surface an error, not be swallowed")
	}
	if !strings.Contains(err.Error(), "dead") {
		t.Fatalf("error must name the host, got %v", err)
	}
}

func TestKeysSyncIsIdempotentOnTheRemoteSide(t *testing.T) {
	var sent []string
	r := recordingRunner{fake: runner.Fake{Out: map[string]string{"alpha": ""}}, log: &sent}
	_ = syncKeyToHost(r, "alpha", "ssh-ed25519 AAA me@box")
	// The remote command must guard the append, or repeated syncs grow the file.
	if !strings.Contains(strings.Join(sent, " "), "grep") {
		t.Fatalf("append must be guarded by a grep so re-syncing is a no-op: %v", sent)
	}
}

// Defect 2 regression, at the command layer: a declined prune must not touch
// the host at all. The shell script blanket-overwrote authorized_keys.
func TestPruneRequiresConfirmationAndChangesNothingWhenDeclined(t *testing.T) {
	var sent []string
	r := recordingRunner{fake: runner.Fake{Out: map[string]string{
		"alpha": "ssh-ed25519 AAA me@box\nssh-ed25519 ZZZ ci@runner",
	}}, log: &sent}
	var sb strings.Builder
	changed, err := pruneHost(&sb, r, "alpha", []string{"ssh-ed25519 AAA me@box"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("a declined prune must report no change")
	}
	for _, c := range sent {
		if strings.Contains(c, ">>") || strings.Contains(c, "sed -i") || strings.Contains(c, "> ~/.ssh") {
			t.Fatalf("declined prune still mutated the host: %q", c)
		}
	}
	if !strings.Contains(sb.String(), "ci@runner") {
		t.Fatalf("prune must SHOW what it would remove:\n%s", sb.String())
	}
}

func TestPruneAppliesOnlyWhenConfirmed(t *testing.T) {
	var sent []string
	r := recordingRunner{fake: runner.Fake{Out: map[string]string{
		"alpha": "ssh-ed25519 AAA me@box\nssh-ed25519 ZZZ ci@runner",
	}}, log: &sent}
	var sb strings.Builder
	changed, err := pruneHost(&sb, r, "alpha", []string{"ssh-ed25519 AAA me@box"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("a confirmed prune should report a change")
	}
}

func TestPruneIsANoOpWhenNothingForeign(t *testing.T) {
	var sent []string
	r := recordingRunner{fake: runner.Fake{Out: map[string]string{
		"alpha": "ssh-ed25519 AAA me@box",
	}}, log: &sent}
	var sb strings.Builder
	changed, err := pruneHost(&sb, r, "alpha", []string{"ssh-ed25519 AAA me@box"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("nothing foreign means nothing to do, even when confirmed")
	}
}
