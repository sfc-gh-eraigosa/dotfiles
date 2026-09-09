package cmd

import (
	"strings"
	"testing"
)

// F20a — THE gate that makes a session-lived credential acceptable. The old
// design forgot the secret between waves; the new one keeps it, so the
// compensating control is that every wave shows what it is about to apply and
// the secret is never legible in any frame.
func TestConfirmStripShowsTheAnswersWithTheSecretMasked(t *testing.T) {
	m := testModel("a", "b")
	m2, _ := send(m, "u")                           // form
	m3, _ := send(m2, "s", "e", "c", "r", "e", "t") // type it
	m4 := commitForm(m3)

	if m4.mode != modeConfirm {
		t.Fatalf("expected the confirm strip, mode=%v", m4.mode)
	}
	view := stripANSI(m4.View())

	if strings.Contains(view, "secret") {
		t.Fatalf("the credential must never be legible in a frame:\n%s", view)
	}
	// Each answer is labelled on its own now, rather than run together behind
	// a single "answers:" prefix — the intent is unchanged: the operator can
	// see everything that is about to be applied, at the gate.
	for _, want := range []string{"sudo", "windows", "gemini"} {
		if !strings.Contains(view, want) {
			t.Fatalf("the confirm dialog must state %q:\n%s", want, view)
		}
	}
	if !strings.Contains(view, strings.Repeat("•", 6)) {
		t.Fatalf("expected a 6-character mask on the confirm strip:\n%s", view)
	}
}

// The summary has to survive into later waves too — that is the wave where the
// operator can no longer remember what they typed.
func TestConfirmSummaryAppearsOnARememberedWave(t *testing.T) {
	m := testModel("a", "b")
	m.ans = remembered()

	m2, _ := send(m, "u")
	view := stripANSI(m2.View())

	// The values still have to be visible at the gate; only the single
	// "answers:" prefix went away when each got its own labelled cell.
	for _, want := range []string{"windows y", "gemini keep"} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirm summary missing %q:\n%s", want, view)
		}
	}
}

// F20b — `e` reopens the form pre-filled, so a remembered set can be corrected
// without being thrown away first.
func TestEditReopensThePrefilledForm(t *testing.T) {
	m := testModel("a", "b")
	m.ans = remembered()

	m2, _ := send(m, "u") // straight to confirm
	m3, _ := send(m2, "e")

	if m3.mode != modeAnswers {
		t.Fatalf("e must reopen the form, mode=%v", m3.mode)
	}
	if m3.ans.secretLen() == 0 || m3.ans.windows != "y" {
		t.Fatalf("the form must be PRE-FILLED, not cleared: %+v", m3.ans)
	}
}

// F20c — because esc no longer forgets, forgetting must be possible on purpose.
func TestForgetClearsEverything(t *testing.T) {
	m := testModel("a", "b")
	m.ans = remembered()

	m2, _ := send(m, "F")
	if m2.ans.secretLen() != 0 || m2.ans.windows != "" || m2.ans.gemini != "" {
		t.Fatalf("F must clear every answer including the credential, got %+v", m2.ans)
	}

	// And the next wave is back to a first-wave experience.
	m3, _ := send(m2, "u")
	if m3.mode != modeAnswers {
		t.Fatalf("after forgetting, u must open the form again, mode=%v", m3.mode)
	}
}

// F20c — 'F' is a literal character while typing. Forgetting the credential
// because the operator typed an F into a search or a password would be absurd.
func TestForgetDoesNotFireWhileTyping(t *testing.T) {
	t.Run("in the answer form", func(t *testing.T) {
		m := testModel("a")
		m2, _ := send(m, "u")
		m3, _ := send(m2, "F", "F")
		if m3.ans.secretLen() != 2 {
			t.Fatalf("F in the credential field is text, got secretLen=%d", m3.ans.secretLen())
		}
	})

	t.Run("in search", func(t *testing.T) {
		m := testModel("a")
		m.ans = remembered()
		m2, _ := send(m, "/")
		m3, _ := send(m2, "F")
		if m3.ans.secretLen() == 0 {
			t.Fatal("F while searching is text, not a command")
		}
		if m3.search.input != "F" {
			t.Fatalf("search input = %q, want %q", m3.search.input, "F")
		}
	})
}

// F20d — keyHelp is the only list; anything missing from it is invisible.
func TestHelpListsTheNewKeys(t *testing.T) {
	var keys []string
	for _, h := range keyHelp {
		keys = append(keys, h.keys)
	}
	joined := strings.Join(keys, " | ")
	for _, want := range []string{"a", "F"} {
		found := false
		for _, k := range keys {
			if strings.TrimSpace(k) == want {
				found = true
			}
		}
		if !found {
			t.Errorf("key %q missing from keyHelp: %s", want, joined)
		}
	}
}

// F22a — select-all over an unfiltered list, and the same key clears it.
func TestSelectAllTogglesEveryRow(t *testing.T) {
	m := testModel("a", "b", "c")

	m2, _ := send(m, "a")
	if len(m2.selected) != 3 {
		t.Fatalf("a must select every row, got %d", len(m2.selected))
	}

	m3, _ := send(m2, "a")
	if len(m3.selected) != 0 {
		t.Fatalf("a again must clear the selection, got %d", len(m3.selected))
	}
}

// F22b — the composable workflow: /pattern -> a -> u targets exactly the hosts
// that matched, which is how the branch column becomes actionable.
func TestSelectAllRespectsTheActiveFilter(t *testing.T) {
	m := testModel("web-01", "web-02", "db-01")
	m.setRow(Row{Alias: "web-01", Class: "up-to-date"})
	m.setRow(Row{Alias: "web-02", Class: "up-to-date"})
	m.setRow(Row{Alias: "db-01", Class: "up-to-date"})
	m.search.input = "web"
	m.compileSearch()
	m.search.committed = true

	m2, _ := send(m, "a")

	if len(m2.selected) != 2 {
		t.Fatalf("expected only the 2 matching hosts, got %d: %v", len(m2.selected), m2.selected)
	}
	if m2.selected["db-01"] {
		t.Fatalf("a non-matching host must not be selected: %v", m2.selected)
	}
}

// F22c — a partial selection fills in rather than toggling off, so `a` never
// destroys work the operator already did.
func TestSelectAllCompletesAPartialSelection(t *testing.T) {
	m := testModel("a", "b", "c")
	m.selected = map[string]bool{"a": true}

	m2, _ := send(m, "a")
	if len(m2.selected) != 3 {
		t.Fatalf("a partial selection must be completed, got %v", m2.selected)
	}
}
