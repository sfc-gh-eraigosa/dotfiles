package cmd

import (
	"path/filepath"

	applog "github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/log"
)

// logTool is fleet's name in the shared log driver. It selects
// $FLEET_LOG_FILE / $FLEET_LOG_LEVEL and the on-disk location.
const logTool = "fleet"

func init() { applog.SetDefaultTool(logTool) }

// fleetLogDir is where per-run install captures go. The precedence is the
// driver's, so fleet's logs sit beside every other tool's.
func fleetLogDir() string {
	s := applog.StateDir(logTool)
	if s == "" {
		return ""
	}
	return filepath.Join(s, "logs")
}
