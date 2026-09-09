package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// canary is the sentinel we hunt for in serialised output — it stands in for
// the operator's sudo credential.
const canary = "hunter2-do-not-persist"

func populated() answers {
	var a answers
	a.appendSecret(canary)
	a.windows, a.gemini = "s", "keep"
	return a
}

// F21a — THE invariant of this file. The credential's lifetime was widened from
// one wave to one session; it must not widen to "forever on disk". Assert on
// the actual bytes, because a future field rename could quietly start
// including it.
func TestSavedAnswersNeverContainTheCredential(t *testing.T) {
	path := filepath.Join(t.TempDir(), "answers.json")
	if err := saveAnswers(path, populated()); err != nil {
		t.Fatalf("saveAnswers: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if strings.Contains(string(raw), canary) {
		t.Fatalf("the credential reached disk:\n%s", raw)
	}
	// The non-secret preferences are the whole point of persisting.
	for _, want := range []string{`"windows"`, `"s"`, `"gemini"`, `"keep"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("expected %s in the saved file:\n%s", want, raw)
		}
	}
}

// A file holding preferences still deserves owner-only permissions: it reveals
// which hosts get which install behaviour.
func TestSavedAnswersFileIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "answers.json")
	if err := saveAnswers(path, populated()); err != nil {
		t.Fatalf("saveAnswers: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("mode = %04o, want 0600", perm)
	}
}

func TestSaveCreatesTheParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeper", "answers.json")
	if err := saveAnswers(path, populated()); err != nil {
		t.Fatalf("saveAnswers must create its directory: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not written: %v", err)
	}
}

// F21b — round-trip restores the preferences and ONLY the preferences.
func TestLoadRestoresPreferencesButNeverACredential(t *testing.T) {
	path := filepath.Join(t.TempDir(), "answers.json")
	if err := saveAnswers(path, populated()); err != nil {
		t.Fatalf("saveAnswers: %v", err)
	}

	got := loadAnswers(path)
	if got.windows != "s" || got.gemini != "keep" {
		t.Fatalf("preferences lost in round-trip: %+v", got)
	}
	if got.secretLen() != 0 {
		t.Fatal("load must never populate the credential")
	}
}

// F21b — hostile input: someone hand-edits a credential into the file, hoping
// fleet will adopt it. It must be ignored, not honoured.
func TestLoadIgnoresACredentialPlantedInTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "answers.json")
	planted := `{"windows":"s","gemini":"keep",` +
		`"sudo":"` + canary + `",` +
		`"sudoSecret":"` + canary + `",` +
		`"password":"` + canary + `"}`
	if err := os.WriteFile(path, []byte(planted), 0o600); err != nil {
		t.Fatal(err)
	}

	got := loadAnswers(path)
	if got.secretLen() != 0 {
		t.Fatal("a credential planted in the file must never be adopted")
	}
	if got.windows != "s" {
		t.Fatalf("the legitimate fields should still load: %+v", got)
	}
}

// F21c — a bad file must never take the dashboard down with it. The answers
// file is a convenience; losing it costs a retype, not a session.
func TestLoadDegradesGracefully(t *testing.T) {
	dir := t.TempDir()

	corrupt := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corrupt, []byte("{not json at all"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, path string
	}{
		{"missing file", filepath.Join(dir, "nope.json")},
		{"corrupt json", corrupt},
		{"a directory where a file should be", dir},
		{"empty path", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := loadAnswers(tc.path)
			if got.remembered() {
				t.Fatalf("expected nothing remembered, got %+v", got)
			}
		})
	}
}

// Regression: newTUIModel used to resolve answersPath() itself, so every test
// that committed the answer form wrote into the developer's REAL
// ~/.config/fleet — polluting a home directory and making later tests depend on
// earlier ones. The path is injected now, and an empty one means "no
// persistence", which is what every test gets by default.
func TestModelDoesNotPersistWithoutAnInjectedPath(t *testing.T) {
	m := testModel("a")
	if m.ansPath != "" {
		t.Fatalf("a bare model must not carry a persistence path, got %q", m.ansPath)
	}

	// Committing the form must be a no-op on disk rather than a panic or a
	// write to some default location.
	m2, _ := send(m, "u")
	m3, _ := send(m2, "p", "w")
	m4 := commitForm(m3)
	if m4.mode != modeConfirm {
		t.Fatalf("form should still commit without persistence, mode=%v", m4.mode)
	}
}

// The full loop the operator actually experiences, through an injected path.
func TestPreferencesRoundTripThroughTheModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "answers.json")

	m := testModel("a")
	m.ansPath = path
	m2, _ := send(m, "u")
	m3, _ := send(m2, "p", "w")         // credential
	m4, _ := send(m3, "enter")          // -> windows field
	m5, _ := send(m4, "s")              // windows = s
	m6, _ := send(m5, "enter")          // -> gemini field
	m7, _ := send(m6, "k")              // gemini = keep
	m8, _ := send(m7, "enter", "enter") // through the reset field -> confirm (and save)

	if m8.mode != modeConfirm {
		t.Fatalf("expected confirm, mode=%v", m8.mode)
	}

	// A NEW session picks the preferences up — but never the credential.
	next := loadAnswers(path)
	if next.windows != "s" || next.gemini != "keep" {
		t.Fatalf("preferences did not survive to the next session: %+v", next)
	}
	if next.secretLen() != 0 {
		t.Fatal("the credential must not survive the session")
	}

	// F forgets for real: a restart must not resurrect what was discarded.
	m9, _ := send(m8, "n")  // leave confirm
	m10, _ := send(m9, "F") // forget
	if m10.ans.remembered() {
		t.Fatalf("F must clear the session answers, got %+v", m10.ans)
	}
	if loadAnswers(path).remembered() {
		t.Fatal("F must also remove the saved preferences, or a restart brings them back")
	}
}

// The default location is under the user's config dir, not somewhere arbitrary.
func TestAnswersPathIsUnderTheConfigDir(t *testing.T) {
	p := answersPath()
	if p == "" {
		t.Fatal("answersPath must resolve to something")
	}
	if !strings.Contains(filepath.ToSlash(p), "fleet/answers.json") {
		t.Fatalf("unexpected answers path: %q", p)
	}
}
