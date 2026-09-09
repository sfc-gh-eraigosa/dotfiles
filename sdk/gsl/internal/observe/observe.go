// Package observe is gsl's handle on the shared logging driver.
//
// It used to BE the implementation — logrus plus lumberjack plus path
// resolution, about 150 lines. That implementation was generalized into
// sdk/libs/log so every tool gets it instead of each growing its own; this
// package is now a thin delegation so gsl's call sites and its environment
// contract ($GSL_LOG_FILE, $GSL_LOG_LEVEL) are unchanged.
//
// New code should prefer importing sdk/libs/log directly. This exists so the
// migration did not have to touch forty call sites at once.
package observe

import (
	"github.com/sirupsen/logrus"

	applog "github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/log"
)

// tool names gsl to the driver. It is what makes $GSL_LOG_FILE and
// $GSL_LOG_LEVEL the variables gsl has always used.
const tool = "gsl"

func init() { applog.SetDefaultTool(tool) }

// Options is the driver's, aliased so existing observe.Options{…} literals
// keep compiling.
type Options = applog.Options

// New builds a logger, defaulting the tool to gsl so path and env resolution
// behave exactly as they did.
func New(opts Options) *logrus.Logger {
	if opts.Tool == "" {
		opts.Tool = tool
	}
	return applog.New(opts)
}

// Default is the process-wide gsl logger.
func Default() *logrus.Logger { return applog.Default() }

// ResolveLogPath returns gsl's log file location.
func ResolveLogPath() string { return applog.ResolvePath(tool) }

// ResetDefaultForTest rearms the lazy singleton. Production code MUST NOT
// call this.
func ResetDefaultForTest() {
	applog.SetDefaultTool(tool)
	applog.ResetDefaultForTest()
}
