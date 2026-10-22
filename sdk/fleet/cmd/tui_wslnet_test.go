package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// WSLNET_ANSWER (spec U5): fleet always answers install.sh's WSL networking
// prompt so an unattended wave never stalls. The default is "skip".

func TestEnvPrefixAlwaysCarriesWslnetDefaultingToSkip(t *testing.T) {
	if got, want := (answers{}).envPrefix(), "export WSLNET_ANSWER=skip; "; got != want {
		t.Fatalf("envPrefix() = %q, want %q", got, want)
	}
	got := answers{wslnet: "nat", windows: "y"}.envPrefix()
	for _, want := range []string{"WSLNET_ANSWER=nat", "WINSETUP_ANSWER=y"} {
		if !strings.Contains(got, want) {
			t.Fatalf("envPrefix() = %q, missing %q", got, want)
		}
	}
	if !strings.HasPrefix(got, "export ") || !strings.HasSuffix(got, "; ") {
		t.Fatalf("envPrefix() must be an export statement, got %q", got)
	}
}

func TestHandoffEnvCarriesWslnetDefaultSkip(t *testing.T) {
	joined := strings.Join(handoffEnv(answers{}), "\n")
	if !strings.Contains(joined, "WSLNET_ANSWER=skip") {
		t.Fatal("handoff env must default WSLNET_ANSWER=skip")
	}
	joined = strings.Join(handoffEnv(answers{wslnet: "bridged"}), "\n")
	if !strings.Contains(joined, "WSLNET_ANSWER=bridged") {
		t.Fatal("handoff env must carry the chosen mode")
	}
}

func TestWslnetCountsAsRemembered(t *testing.T) {
	if !(answers{wslnet: "off"}).remembered() {
		t.Fatal("a wslnet answer is a remembered answer")
	}
}

func TestWslnetStoreRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "answers.json")
	if err := saveAnswers(path, answers{wslnet: "mirrored"}); err != nil {
		t.Fatal(err)
	}
	if got := loadAnswers(path); got.wslnet != "mirrored" {
		t.Fatalf("wslnet lost in round-trip: %+v", got)
	}
}

func TestWslnetFieldCyclesAndHasDirectKeys(t *testing.T) {
	m := testModel("a")
	m.mode = modeAnswers
	m.ansField = fieldWslnet
	for _, want := range []string{"off", "nat", "bridged", "mirrored", ""} {
		m, _ = send(m, "right")
		if m.ans.wslnet != want {
			t.Fatalf("after right: wslnet = %q, want %q", m.ans.wslnet, want)
		}
	}
	for _, kw := range [][2]string{{"o", "off"}, {"n", "nat"}, {"b", "bridged"}, {"m", "mirrored"}} {
		m, _ = send(m, kw[0])
		if m.ans.wslnet != kw[1] {
			t.Fatalf("key %q: wslnet = %q, want %q", kw[0], m.ans.wslnet, kw[1])
		}
	}
	m, _ = send(m, "left")
	if m.ans.wslnet != "bridged" {
		t.Fatalf("left from mirrored should give bridged, got %q", m.ans.wslnet)
	}
}

func TestWslnetFormRowAndStrip(t *testing.T) {
	m := testModel("a")
	m.mode = modeAnswers
	if v := m.answersView(); !strings.Contains(v, "(unset = skip, record nothing)") {
		t.Fatalf("form must label the unset wslnet row:\n%s", v)
	}
	m.ans.wslnet = "nat"
	m.mode = modeConfirm
	if v := m.confirmView(); !strings.Contains(v, "wslnet") {
		t.Fatalf("confirm strip must show wslnet:\n%s", v)
	}
}

func TestLocalAnswerPreambleWslnet(t *testing.T) {
	t.Setenv("WSLNET_ANSWER", "")
	if got := localAnswerPreamble(updplan.Step{}); !strings.Contains(got, "WSLNET_ANSWER=skip") {
		t.Fatalf("preamble must default to skip, got %q", got)
	}
	t.Setenv("WSLNET_ANSWER", "mirrored")
	if got := localAnswerPreamble(updplan.Step{}); !strings.Contains(got, "WSLNET_ANSWER=mirrored") {
		t.Fatalf("preamble must honour the env, got %q", got)
	}
}

// The answer is spliced into a remote shell string, so only install.sh's own
// modes travel; anything else (a stray local env value, a hand-edited
// answers.json) degrades to skip instead of reaching the host verbatim.
func TestWslnetAnswerIsAllowlisted(t *testing.T) {
	t.Setenv(envWslnetAnswer, "nat; touch /tmp/pwned")
	if got := localAnswerPreamble(updplan.Step{Kind: updplan.KindRun}); got != "export WSLNET_ANSWER=skip; " {
		t.Fatalf("an unlisted local WSLNET_ANSWER must become skip, got %q", got)
	}
	t.Setenv(envWslnetAnswer, "mirrored")
	if got := localAnswerPreamble(updplan.Step{Kind: updplan.KindRun}); got != "export WSLNET_ANSWER=mirrored; " {
		t.Fatalf("a listed local WSLNET_ANSWER must pass through, got %q", got)
	}
	if got := (answers{wslnet: "$(id)"}).envPrefix(); got != "export WSLNET_ANSWER=skip; " {
		t.Fatalf("an unlisted stored mode must become skip, got %q", got)
	}
	path := filepath.Join(t.TempDir(), "answers.json")
	if err := saveAnswers(path, answers{wslnet: "$(id)"}); err != nil {
		t.Fatal(err)
	}
	if got := loadAnswers(path).wslnet; got != "" {
		t.Fatalf("an unlisted persisted mode must load as unset, got %q", got)
	}
}
