package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSaveSession_PrivateModes: session records live under $HOME and are the
// user's private state — 0600 file in a 0700 directory (security-scan-green).
func TestSaveSession_PrivateModes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	s := Session{SessionID: "s1", AgentName: "a", Status: "running", StartTime: time.Now()}
	if err := SaveSession(s); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	dir := filepath.Join(home, sessionsDir)
	for p, want := range map[string]os.FileMode{filepath.Join(dir, "s1.json"): 0o600, dir: 0o700} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if got := fi.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o; want %o", p, got, want)
		}
	}
}
