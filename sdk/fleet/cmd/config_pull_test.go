package cmd

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/cfgplan"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
)

func TestPullPlanReadsTheSourceConfigOverTheRunnerSeam(t *testing.T) {
	r := runner.Fake{Out: map[string]string{"src": "Host fresh  #fleet\n    HostName 10.0.0.2\n"}}
	p, err := pullPlan(r, "src", "", cfgplan.Opts{Marker: "#fleet", Source: "src"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Changes) != 1 || p.Changes[0].Kind != cfgplan.Add {
		t.Fatalf("plan = %+v", p.Changes)
	}
}

// A trust failure must not be reported as a network one — the lesson already
// pinned in internal/sshfail.
func TestPullReportsATrustFailureAsSuch(t *testing.T) {
	_, sshErr := exec.Command("sh", "-c", `printf 'Host key verification failed.' >&2; exit 255`).Output()
	r := runner.Fake{Err: map[string]error{"src": sshErr}}
	_, err := pullPlan(r, "src", "", cfgplan.Opts{Marker: "#fleet"})
	if err == nil || !strings.Contains(err.Error(), "host key unverified") {
		t.Fatalf("err = %v, want it to name the trust fault", err)
	}
}

// Reading a host must never write to it — the discipline the wake ladder holds.
func TestPullOnlyEverReadsTheSource(t *testing.T) {
	var argv []string
	r := recordingRunner{
		fake: runner.Fake{Out: map[string]string{"src": "Host a  #fleet\n    HostName 10.0.0.1\n"}},
		log:  &argv,
	}
	if _, err := pullPlan(r, "src", "", cfgplan.Opts{Marker: "#fleet"}); err != nil {
		t.Fatal(err)
	}
	// Stderr suppression is not a mutation, so it is not evidence of one.
	// Everything else that could write must stay absent.
	joined := strings.ReplaceAll(strings.Join(argv, " "), "2>/dev/null", "")
	for _, banned := range []string{">", "tee", "cp ", "mv ", "rm ", "sed -i", "install", "chmod"} {
		if strings.Contains(joined, banned) {
			t.Fatalf("pull issued a mutating command: %q", joined)
		}
	}
}

func TestPullReportsAnEmptyRemoteConfig(t *testing.T) {
	r := runner.Fake{Out: map[string]string{"src": "  \n"}}
	if _, err := pullPlan(r, "src", "", cfgplan.Opts{Marker: "#fleet"}); err == nil {
		t.Fatal("want an error when the source has no readable config")
	}
}
