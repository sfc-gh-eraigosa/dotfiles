package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/app"
)

// The README's key table must list every key the TUI binds (one table, no
// drift).
func TestReadmeKeyTableMatchesBindings(t *testing.T) {
	b, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatal(err)
	}
	readme := string(b)
	start := strings.Index(readme, "## Keys")
	if start < 0 {
		t.Fatal("README has no Keys section")
	}
	table := readme[start:]
	if end := strings.Index(table[3:], "\n## "); end > 0 {
		table = table[:end+3]
	}
	for _, binding := range app.Keys() {
		for _, k := range binding.Keys {
			if !strings.Contains(table, "`"+k+"`") {
				t.Errorf("README Keys table is missing `%s` (%s)", k, binding.Help)
			}
		}
	}
}
