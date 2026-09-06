package gh

import (
	"errors"
	"testing"
)

// StubAskGHForTests makes the `gh auth token` fallback fail for the rest of
// the test, so a test in another package cannot pick up this machine's real
// credential or spawn a process. It is in the non-test build so packages
// above internal/gh can call it.
func StubAskGHForTests(t *testing.T) {
	t.Helper()
	old := askGH
	askGH = func() (string, error) { return "", errors.New("gh: stubbed out in tests") }
	t.Cleanup(func() { askGH = old })
}
