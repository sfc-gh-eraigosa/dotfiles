package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/cfgplan"
)

// missingIdentities reports imported IdentityFile paths that do not exist
// locally.
//
// An imported IdentityFile is only a PATH: if the key is not on this machine
// the alias looks configured and fails at connect time, which is exactly the
// kind of silent breakage this tool exists to surface. `exists` is injected so
// the check is testable and so this can never read key MATERIAL — it only ever
// asks whether a path is present.
func missingIdentities(p cfgplan.Plan, exists func(string) bool) []string {
	var out []string
	for _, c := range p.Changes {
		if c.Kind != cfgplan.Add && c.Kind != cfgplan.Update {
			continue
		}
		if c.Host.Identity == "" || exists(c.Host.Identity) {
			continue
		}
		out = append(out, fmt.Sprintf("%s → %s (missing)", c.Alias, c.Host.Identity))
	}
	return out
}

// localFileExists expands a leading ~ and reports presence only. It never opens
// the file: no private key is read, here or anywhere else in fleet.
func localFileExists(path string) bool {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	_, err := os.Stat(path)
	return err == nil
}
