package cmd

import (
	"path/filepath"

	applog "github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/log"
)

// logTool is fleet's name in the shared log driver. It selects
// $FLEET_LOG_FILE / $FLEET_LOG_LEVEL and the on-disk location.
const logTool = "fleet"

func init() { applog.SetDefaultTool(logTool) }

// captureKeep is how many captures are retained PER HOST. libs/log defaults
// to 200, which is far more scrollback than an operator ever reads and is how
// ~/.local/state/fleet/logs reached several hundred files; 50 runs answers
// "what changed since this host last worked" while keeping the directory
// listable. Retention is per subject, so a rarely-updated host never has its
// history evicted by a busy one.
const captureKeep = 50

// fleetLogDir is where per-run install captures go. The precedence is the
// driver's, so fleet's logs sit beside every other tool's.
func fleetLogDir() string {
	s := applog.StateDir(logTool)
	if s == "" {
		return ""
	}
	return filepath.Join(s, "logs")
}
