// Package logging wires tmux-mgr's output into the shared log driver.
//
// tmux-mgr's ~30 log.Printf calls already went to a FILE, not the terminal:
// cmd/root.go opened ~/.config/tmux-mgr/tmux-mgr.log and called
// log.SetOutput on it. So these are diagnostics, not user output, and moving
// them to the driver changes nothing an operator sees — while fixing three
// things the hand-rolled version got wrong:
//
//   - it never rotated. The file grew without bound for the life of the
//     install.
//   - it lived in ~/.config, which is for configuration. Logs are state.
//   - it had no level control and no env override.
//
// Call sites are deliberately untouched: pointing the standard logger at the
// driver migrates all thirty at once, with no chance of changing one of them
// by accident.
package logging

import (
	stdlog "log"

	"github.com/sirupsen/logrus"

	applog "github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/log"
)

// Tool is tmux-mgr's name in the driver; it selects $TMUX_MGR_LOG_FILE and
// $TMUX_MGR_LOG_LEVEL (hyphens become underscores).
const Tool = "tmux-mgr"

// Init routes the standard library's logger into the driver. It never fails:
// if the file cannot be opened the driver discards, and the tool runs on.
//
// The stdlib date/time prefix is dropped because logrus stamps every record
// itself — leaving it produces two timestamps per line.
func Init() {
	applog.SetDefaultTool(Tool)
	stdlog.SetFlags(0)
	stdlog.SetOutput(applog.Default().WriterLevel(logrus.InfoLevel))
}

// L is the structured logger, for new code that should log fields rather
// than a formatted sentence.
func L() *logrus.Logger { return applog.Default() }
