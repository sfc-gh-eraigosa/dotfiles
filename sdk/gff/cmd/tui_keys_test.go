package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The footer, the help overlay, README, and --help all derive from gffKeys;
// this pins the --help side so a new binding cannot ship undocumented.
func TestTUIHelpListsVimSearchAndCommandKeys(t *testing.T) {
	for _, want := range []string{"j/k", "h/l", "gg/G", "ctrl+d", "/ ", "n/N", ":set", ":unset", ":q", "? help", "libs/tui/GUIDE.md"} {
		assert.Contains(t, tuiCmd.Long, want, "gff tui --help must mention %q", want)
	}
	assert.NotContains(t, tuiCmd.Long, "h help", "h no longer opens help")
}
