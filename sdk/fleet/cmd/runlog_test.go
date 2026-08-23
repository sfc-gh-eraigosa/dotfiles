package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
)

// fleet no longer owns the file mechanics — location, mode, header format,
// timestamps and retention are libs/log's, and tested there. What fleet still
// owns is WHAT a run is about, so that is what these assert.

func TestUpdateIsCapturedWithItsSubject(t *testing.T) {
	dir := t.TempDir()
	r := runner.Fake{Out: map[string]string{"host-a": "Installing sops...\ndone"}}

	st := beginStream("host-a", "main", answers{}, r, dir)().(streamStartedMsg).st
	var streamed []string
	for l := range st.lines {
		streamed = append(streamed, l)
	}
	<-st.done

	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatalf("expected one capture, got %v", files)
	}
	b, _ := os.ReadFile(filepath.Join(dir, files[0].Name()))
	s := string(b)
	for _, want := range []string{"host=host-a", "ref=main", "mode=fast-forward", "Installing sops", "done"} {
		if !strings.Contains(s, want) {
			t.Fatalf("capture missing %q:\n%s", want, s)
		}
	}
	// Teeing must not change what the pane sees.
	if strings.Join(streamed, ",") != "Installing sops...,done" {
		t.Fatalf("the tee altered the stream: %v", streamed)
	}
}

// A forced reset is the destructive mode; the capture has to say so, because
// that is the run someone comes back asking about.
func TestForcedResetIsLabelledInTheCapture(t *testing.T) {
	dir := t.TempDir()
	r := runner.Fake{Out: map[string]string{"h": "x"}}
	st := beginStream("h", "main", answers{reset: "y"}, r, dir)().(streamStartedMsg).st
	for range st.lines {
	}
	<-st.done
	files, _ := os.ReadDir(dir)
	b, _ := os.ReadFile(filepath.Join(dir, files[0].Name()))
	if !strings.Contains(string(b), "mode=FORCE RESET") {
		t.Fatalf("a destructive run must be labelled:\n%s", b)
	}
}

// Losing the capture must never cost the update — the guarantee fleet relies
// on from the driver.
func TestAnUnusableCaptureDirDoesNotBreakTheStream(t *testing.T) {
	r := runner.Fake{Out: map[string]string{"h": "still streams"}}
	for _, dir := range []string{"", "/proc/cannot/mkdir/here"} {
		st := beginStream("h", "main", answers{}, r, dir)().(streamStartedMsg).st
		var got []string
		for l := range st.lines {
			got = append(got, l)
		}
		<-st.done
		if len(got) != 1 || got[0] != "still streams" {
			t.Fatalf("dir %q: the stream must survive, got %v", dir, got)
		}
	}
}

// fleet's captures sit under the shared driver's state dir, beside every
// other tool's, rather than in a location fleet invented.
func TestCaptureDirComesFromTheSharedDriver(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	got := fleetLogDir()
	want := filepath.Join(dir, "fleet", "logs")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
