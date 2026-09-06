package cmd

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/pkg/provider"
)

// fleetBoundRunes returns every single printable rune fleet binds, read from
// keyHelp — the TUI's own single source of truth. Multi-rune names (gg, tab,
// ctrl+d, space) and the arrow glyphs in the help text are not runes a
// provider could declare, so they cannot collide.
func fleetBoundRunes() map[rune]string {
	out := map[rune]string{}
	for _, k := range keyHelp {
		for _, name := range strings.Split(k.keys, " / ") {
			name = strings.TrimSpace(name)
			if name == "space" {
				out[' '] = k.what
				continue
			}
			r, size := utf8.DecodeRuneInString(name)
			if size != len(name) || r == utf8.RuneError || !unicode.IsPrint(r) || r > unicode.MaxASCII {
				continue
			}
			out[r] = k.what
		}
	}
	return out
}

// The mechanical half of the provider key contract: pkg/provider is stdlib-only
// and cannot import fleet's keymap, so it mirrors it. This test is what keeps
// the mirror honest. If it fails, fleet bound a key providers can still take —
// add the rune to provider.ReservedKeys, do not weaken this test.
//
// It exists because the first hand-written version of that list missed six keys
// fleet already bound, `l` (the log pane) and `s` (ssh) among them.
func TestEveryFleetKeyIsReservedAgainstProviders(t *testing.T) {
	for r, what := range fleetBoundRunes() {
		if !provider.ReservedKeys[r] {
			t.Errorf("fleet binds %q (%s) but a provider may declare it — add it to provider.ReservedKeys", string(r), what)
		}
	}
}

// The mirror must not drift the other way either: a rune reserved against
// providers but bound by nobody is either a key held deliberately for a plan,
// or a leftover. Both are fine, but they must be named, so the list stays
// reviewable instead of accumulating.
func TestEveryReservedKeyIsBoundOrDeliberatelyHeld(t *testing.T) {
	held := map[rune]string{
		'h': "navigation real estate — the shared keymap's page-left; never help, which is ?",
		':': "the command line, per the sdk TUI guide",
		'g': "only ever half of the gg chord, so the rune itself must stay free",
		't': "provider.TunnelKey — every Tunnel action declares it",
		'T': "stop every bridge on the level's host",
		'q': "quit (keyHelp spells it, but so does every sdk TUI)",
	}
	bound := fleetBoundRunes()
	for r := range provider.ReservedKeys {
		if _, ok := bound[r]; ok {
			continue
		}
		if _, ok := held[r]; !ok {
			t.Errorf("provider.ReservedKeys holds %q but nothing binds it and no reason is recorded — bind it, drop it, or name it here", string(r))
		}
	}
}
