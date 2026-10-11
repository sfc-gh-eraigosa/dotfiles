package cmd

import (
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
	"strings"
	"testing"
	"time"
)

// keyHelp is the single source of truth for both the header strip and the `?`
// overlay, so a binding missing from it ships undiscoverable — exactly how the
// log pane shipped invisible.
func TestConfigKeysAreDeclaredInKeyHelp(t *testing.T) {
	var pull, push bool
	for _, k := range keyHelp {
		switch k.keys {
		case "p":
			pull = true
		case "P":
			push = true
		}
	}
	if !pull || !push {
		t.Fatalf("p=%v P=%v — both must be declared in keyHelp", pull, push)
	}
}

// Every binding letter must be unique, or one silently shadows another.
func TestKeyHelpHasNoDuplicateBindings(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range keyHelp {
		if seen[k.keys] {
			t.Fatalf("duplicate binding %q in keyHelp", k.keys)
		}
		seen[k.keys] = true
	}
}

// A host another async path owns must never be claimed: two paths owning one
// row is the bug class the ownership set exists to prevent.
func TestConfigActionSkipsAHostAnotherPathOwns(t *testing.T) {
	m := newTUIModel(nil, nil, nil, time.Time{}, "", 1, updplan.Default())
	m.cursor = "busy"
	m.waking = map[string]bool{"busy": true}
	if m.canStartConfigAction() {
		t.Fatal("a waking host must not be claimed by a config action")
	}
	m.waking = map[string]bool{}
	if !m.canStartConfigAction() {
		t.Fatal("an idle cursor host must be actionable")
	}
	m.cursor = ""
	if m.canStartConfigAction() {
		t.Fatal("no cursor host means nothing to act on")
	}
}

// The TUI must delegate to the CLI verb rather than reimplement the transfer,
// so every guard (loopback, self-retarget, validation, confirmation) applies
// identically from both entry points.
func TestConfigActionDelegatesToTheOneWayCliVerb(t *testing.T) {
	for _, tc := range []struct{ dir, want string }{
		{"pull", "config pull host-a"},
		{"push", "config push host-a"},
	} {
		got := strings.Join(configVerbArgs(tc.dir, "host-a"), " ")
		if got != tc.want {
			t.Fatalf("configVerbArgs(%q) = %q, want %q", tc.dir, got, tc.want)
		}
	}
}

// Authorizing a key is offered ONLY where it can help: a host that answered and
// refused us. Offering it for an unreachable host would be misleading — there
// is nothing there to authorize — and pointless for one that already works.
func TestAuthorizeIsOfferedOnlyForAnAuthFailedHost(t *testing.T) {
	m := newTUIModel(nil, nil, nil, time.Time{}, "", 1, updplan.Default())
	for _, tc := range []struct {
		class string
		want  bool
	}{
		{"auth-failed", true},
		{"unreachable", false},
		{"up-to-date", false},
		{"polling", false},
	} {
		m.setRow(Row{Alias: "h", Class: tc.class})
		m.cursor = "h"
		if got := m.canAuthorize(); got != tc.want {
			t.Errorf("class %q: canAuthorize = %v, want %v", tc.class, got, tc.want)
		}
	}
}

// The password must reach ssh's own prompt on the real terminal, never fleet:
// ssh reads from /dev/tty by design precisely so it cannot be piped, and the
// module bans a secret in argv or the environment.
func TestAuthorizeHandsTheTerminalToSshCopyIdAndCarriesNoSecret(t *testing.T) {
	argv := authorizeArgs("host-a")
	joined := strings.Join(argv, " ")
	if !strings.HasPrefix(joined, "ssh-copy-id ") {
		t.Fatalf("argv = %q, want ssh-copy-id", joined)
	}
	if !strings.Contains(joined, ".pub") {
		t.Fatalf("argv = %q, must offer a PUBLIC key", joined)
	}
	for _, banned := range []string{"password", "sshpass", "SSH_ASKPASS", "-o PreferredAuthentications"} {
		if strings.Contains(joined, banned) {
			t.Fatalf("argv carries a credential mechanism it must not: %q", joined)
		}
	}
	if !strings.HasSuffix(joined, " host-a") {
		t.Fatalf("argv = %q, want the alias last", joined)
	}
}

func TestAuthorizeKeyIsDeclaredInKeyHelp(t *testing.T) {
	for _, k := range keyHelp {
		if k.keys == "A" {
			return
		}
	}
	t.Fatal("A must be declared in keyHelp or it ships undiscoverable")
}

// A host-key fault and a credential fault have different fixes, so they get
// different keys: T (fleet trust, fixes OUR known_hosts) and A (ssh-copy-id,
// fixes THEIR authorized_keys). ssh-copy-id on a host-key row connects through
// the same known_hosts, is refused, and prints ssh's MITM banner twice.
func TestTrustAndAuthorizeSplitByFault(t *testing.T) {
	m := newTUIModel(nil, nil, nil, time.Time{}, "", 1, updplan.Default())
	for _, tc := range []struct {
		class, note      string
		trust, authorize bool
	}{
		{"auth-failed", "host key CHANGED", true, false},
		{"auth-failed", "host key unverified", true, false},
		{"auth-failed", "permission denied", false, true},
		{"unreachable", "", false, false},
		{"up-to-date", "", false, false},
	} {
		m.setRow(Row{Alias: "h", Class: tc.class, Note: tc.note})
		m.cursor = "h"
		if got := m.canTrust(); got != tc.trust {
			t.Errorf("%s/%q: canTrust = %v", tc.class, tc.note, got)
		}
		if got := m.canAuthorize(); got != tc.authorize {
			t.Errorf("%s/%q: canAuthorize = %v", tc.class, tc.note, got)
		}
	}
}

// T self-execs the CLI verb, so the typed confirmation, backup and audit
// apply identically from either entry point.
func TestTrustArgsRunTheCLIVerb(t *testing.T) {
	if got := strings.Join(trustArgs("/x/fleet", "host-a"), " "); got != "/x/fleet trust host-a" {
		t.Fatalf("argv = %q", got)
	}
}

func TestTKeyHandsOverOnlyOnAHostKeyRow(t *testing.T) {
	m := newTUIModel(nil, nil, nil, time.Time{}, "", 1, updplan.Default())
	m.setRow(Row{Alias: "h", Class: "auth-failed", Note: "host key CHANGED"})
	m.cursor = "h"
	if _, cmd := m.Update(key("T")); cmd == nil {
		t.Fatal("T on a host-key row must hand the terminal to fleet trust")
	}
	m.setRow(Row{Alias: "h", Class: "auth-failed", Note: "permission denied"})
	if _, cmd := m.Update(key("T")); cmd != nil {
		t.Fatal("T must do nothing where trusting a key cannot help")
	}
}

func TestTrustKeyIsDeclaredInKeyHelp(t *testing.T) {
	for _, k := range keyHelp {
		if k.keys == "T" {
			return
		}
	}
	t.Fatal("T must be declared in keyHelp or it ships undiscoverable")
}

// The tip is what makes T discoverable: the moment the cursor sits on a
// host-key row, the status line names the problem and the key.
func TestStatusLineTipsTrustOnAHostKeyRow(t *testing.T) {
	m := newTUIModel(nil, nil, nil, time.Time{}, "", 1, updplan.Default())
	m.setRow(Row{Alias: "gig", Class: "auth-failed", Note: "host key CHANGED"})
	m.cursor = "gig"
	got := m.statusView()
	for _, want := range []string{"host key CHANGED", "gig", "T"} {
		if !strings.Contains(got, want) {
			t.Fatalf("status line missing %q: %q", want, got)
		}
	}
	m.setRow(Row{Alias: "gig", Class: "up-to-date"})
	if strings.Contains(m.statusView(), "trust") {
		t.Fatal("a healthy row must not carry the tip")
	}
}
