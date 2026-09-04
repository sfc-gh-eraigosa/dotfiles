package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// This file persists the operator's install.sh prompt preferences between
// sessions — and deliberately NOT the sudo credential.
//
// Sticky answers widened the credential's lifetime from one wave to one
// process, which is the trade the operator asked for. Widening it to "forever,
// on disk" is a different trade nobody asked for, so the on-disk shape simply
// has nowhere to put it: `storedAnswers` has two fields, and the credential is
// not one of them. That is stronger than remembering to strip it, because the
// type cannot express the mistake.
//
// The distinction from the rejected wake cache (design §3.1) is that these are
// STATED preferences, not INFERRED state. `windows: s` means the same thing
// next week; "host X seemed dead" does not.

// storedAnswers is the entire on-disk surface. Adding a credential field here
// would be the bug; `TestSavedAnswersNeverContainTheCredential` asserts on the
// serialised bytes so a rename cannot smuggle one in.
type storedAnswers struct {
	Windows string `json:"windows,omitempty"`
	Gemini  string `json:"gemini,omitempty"`
}

// fleetConfigDir is where fleet's own config files live — answers.json today,
// fleet.yaml (the update plan) from task 19. It follows os.UserConfigDir, so
// it respects XDG_CONFIG_HOME on Linux and the platform convention elsewhere.
func fleetConfigDir() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "fleet")
}

// answersPath is where the preferences live.
func answersPath() string {
	dir := fleetConfigDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "answers.json")
}

// defaultPlanPath is where the update plan lives when neither --file nor gff's
// fleet.update.config selects a location.
func defaultPlanPath() string {
	dir := fleetConfigDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "fleet.yaml")
}

// saveAnswers writes the non-secret preferences, owner-only. The file reveals
// which install behaviour a fleet gets, which is not a secret but is nobody
// else's business either.
func saveAnswers(path string, a answers) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(storedAnswers{Windows: a.windows, Gemini: a.gemini}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

// loadAnswers restores the preferences. It never returns an error: the file is
// a convenience, and a missing or corrupt one costs the operator a retype, not
// a session. The returned value's credential is always empty — unmarshalling
// into storedAnswers means a credential hand-planted in the file has no field
// to land in.
func loadAnswers(path string) answers {
	if path == "" {
		return answers{}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return answers{}
	}
	var s storedAnswers
	if err := json.Unmarshal(b, &s); err != nil {
		return answers{}
	}
	return answers{windows: s.Windows, gemini: s.Gemini}
}
