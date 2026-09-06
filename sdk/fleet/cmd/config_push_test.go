package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/cfgplan"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
)

// The specific way a helpful push becomes unrecoverable: it rewrites how we
// reach the very host we are writing to.
func TestSelfRetargetIsDetected(t *testing.T) {
	p := cfgplan.Plan{Changes: []cfgplan.Change{{
		Alias: "target", Kind: cfgplan.Update,
		Fields: []cfgplan.FieldDelta{{Name: "HostName", From: "10.0.0.1", To: "10.0.0.2"}},
	}}}
	if !selfRetarget(p, "target") {
		t.Fatal("changing the connecting alias must be detected")
	}
	if selfRetarget(p, "other") {
		t.Fatal("an unrelated alias must not trip the guard")
	}
}

// Adding a NEW block for the target is not a retarget — it changes nothing
// about the route we are currently using.
func TestSelfRetargetIgnoresAnAdd(t *testing.T) {
	p := cfgplan.Plan{Changes: []cfgplan.Change{{Alias: "target", Kind: cfgplan.Add}}}
	if selfRetarget(p, "target") {
		t.Fatal("an add must not trip the retarget guard")
	}
}

// failAtRunner fails only the commands containing a marker, so a test can make
// STAGING succeed and VALIDATION fail — the exact sequence that matters, which
// runner.Fake cannot express because its verdict is per host, not per command.
type failAtRunner struct {
	marker string
	log    *[]string
}

func (r failAtRunner) run(argv ...string) (string, error) {
	line := strings.Join(argv, " ")
	*r.log = append(*r.log, line)
	if strings.Contains(line, r.marker) {
		return "", runner.ErrFake
	}
	return "", nil
}
func (r failAtRunner) Run(_ string, a ...string) (string, error)         { return r.run(a...) }
func (r failAtRunner) RunStdin(_, _ string, a ...string) (string, error) { return r.run(a...) }
func (r failAtRunner) RunVia(_, _ string, a ...string) (string, error)   { return r.run(a...) }
func (r failAtRunner) RunInteractive(_ string, a ...string) error        { _, err := r.run(a...); return err }
func (r failAtRunner) RunStream(string, string, ...string) (<-chan string, <-chan error) {
	l, d := make(chan string), make(chan error, 1)
	close(l)
	d <- nil
	return l, d
}
func (r failAtRunner) RunStreamCtx(context.Context, string, string, ...string) (<-chan string, <-chan error) {
	l, d := make(chan string), make(chan error, 1)
	close(l)
	d <- nil
	return l, d
}

// A config that will not parse must never replace a working one.
func TestRemoteInstallValidatesBeforeMoving(t *testing.T) {
	var log []string
	r := failAtRunner{marker: "ssh -F", log: &log} // staging succeeds; validation fails
	err := remoteInstall(r, "target", "Host a\n    HostName 10.0.0.1\n")
	if err == nil {
		t.Fatal("a failed validation must abort the install")
	}
	joined := strings.Join(log, " ")
	if strings.Contains(joined, "mv ") {
		t.Fatalf("install moved the file despite failed validation: %q", joined)
	}
	if !strings.Contains(joined, "rm -f") {
		t.Fatalf("a rejected staging file must be cleaned up: %q", joined)
	}
}

// The backup must exist before the live file is replaced, not after.
func TestRemoteInstallBacksUpBeforeReplacing(t *testing.T) {
	var log []string
	r := failAtRunner{marker: "\x00never", log: &log}
	if err := remoteInstall(r, "target", "Host a\n"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(log, " ")
	iBak, iMove := strings.Index(joined, ".bak-"), strings.Index(joined, "mv ")
	if iBak < 0 || iMove < 0 || iBak > iMove {
		t.Fatalf("backup must precede the move: %q", joined)
	}
}

// The staged file must never be world-readable, even briefly.
func TestRemoteInstallStagesAt0600(t *testing.T) {
	var log []string
	r := failAtRunner{marker: "\x00never", log: &log}
	_ = remoteInstall(r, "target", "Host a\n")
	if !strings.Contains(strings.Join(log, " "), "chmod 600") {
		t.Fatalf("staging must chmod 600: %q", log)
	}
}

// Losing a host here is recoverable only out-of-band, so it must be shouted
// about rather than inferred from silence.
func TestPostPushProbeFlagsATargetThatStoppedAnswering(t *testing.T) {
	r := runner.Fake{Err: map[string]error{"target": runner.ErrFake}}
	err := verifyTarget(r, "target")
	if err == nil || !strings.Contains(err.Error(), "STOPPED ANSWERING") {
		t.Fatalf("err = %v, want a loud failure naming the backup", err)
	}
	if !strings.Contains(err.Error(), ".bak-") {
		t.Fatalf("err = %v, want it to name where the backup is", err)
	}
}

// A push must MERGE into the target's config, never replace it. Applying the
// plan to an empty string would silently delete every host and directive the
// target had that we do not model — the worst possible outcome for a verb whose
// failure mode is losing SSH access.
func TestPushMergesIntoTheTargetConfigRatherThanReplacingIt(t *testing.T) {
	remote := "Host theirs\n    HostName 10.9.9.9\n    ProxyCommand nc %h %p\n"
	local := "Host mine  #fleet\n    HostName 10.0.0.1\n"
	r := runner.Fake{Out: map[string]string{"target": remote}}

	p, remoteText, err := pushPlan(r, "target", local, cfgplan.Opts{Marker: "#fleet"})
	if err != nil {
		t.Fatal(err)
	}
	next, err := p.Apply(remoteText)
	if err != nil {
		t.Fatal(err)
	}
	for _, keep := range []string{"Host theirs", "10.9.9.9", "ProxyCommand nc %h %p"} {
		if !strings.Contains(next, keep) {
			t.Fatalf("push destroyed the target's own config (%q missing):\n%s", keep, next)
		}
	}
	if !strings.Contains(next, "Host mine") {
		t.Fatalf("push did not deliver our host:\n%s", next)
	}
}
