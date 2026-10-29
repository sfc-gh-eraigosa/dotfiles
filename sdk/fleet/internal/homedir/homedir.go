// Package homedir resolves the "~" and "~/…" spellings fleet.yaml and
// ssh_config use for paths on THIS machine. It is the one copy: hostkey and
// the TUI header both used to carry their own.
package homedir

import (
	"path/filepath"
	"strings"
)

// Expand resolves p against home: "~" is home, "~/x" is home/x, and anything
// else (absolute, relative, "~user/…") is returned unchanged.
func Expand(p, home string) string {
	if p == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return p
}
