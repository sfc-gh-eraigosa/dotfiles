package log

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Default file modes. Logs can echo hostnames, paths and command lines, so
// they are owner-readable rather than world-readable.
const (
	dirPerm  = 0o700
	filePerm = 0o600
)

// Options configures a logger. Zero values pick sensible defaults, so
// Options{Tool: "fleet"} is a complete configuration.
type Options struct {
	// Tool names the component ("fleet", "gsl"). It selects the env override
	// ($FLEET_LOG_FILE) and the on-disk location. Required for path
	// resolution; without it Path must be set explicitly.
	Tool string
	// Path overrides the resolved location entirely.
	Path string
	// Level is a logrus level ("debug", "info", "warn", "error"). Empty falls
	// back to $<TOOL>_LOG_LEVEL, then "info".
	Level string
	// Text swaps the JSON formatter for logrus's text one, for a log a human
	// reads directly.
	Text bool

	MaxSizeMB  int // rotation size cap; 0 → 5
	MaxBackups int // rotated files retained; 0 → 3
	MaxAgeDays int // rotated file age; 0 → 7
}

// New builds a logger from opts. It never returns nil and never panics: a
// logger whose file cannot be opened writes to io.Discard, so callers may log
// unconditionally.
func New(opts Options) *logrus.Logger {
	l := logrus.New()
	if opts.Text {
		l.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	} else {
		l.SetFormatter(&logrus.JSONFormatter{TimestampFormat: "2006-01-02T15:04:05.000Z07:00"})
	}
	l.SetLevel(parseLevel(opts.Tool, opts.Level))

	path := opts.Path
	if path == "" {
		path = ResolvePath(opts.Tool)
	}
	if path == "" {
		l.SetOutput(io.Discard)
		return l
	}
	// Probe before handing the path to lumberjack, which would otherwise fail
	// silently on the first write.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, filePerm)
	if err != nil {
		l.SetOutput(io.Discard)
		return l
	}
	_ = f.Close()

	l.SetOutput(&lumberjack.Logger{
		Filename:   path,
		MaxSize:    nonZero(opts.MaxSizeMB, 5),
		MaxBackups: nonZero(opts.MaxBackups, 3),
		MaxAge:     nonZero(opts.MaxAgeDays, 7),
		LocalTime:  true,
		Compress:   true,
	})
	return l
}

// ResolvePath returns the log file for tool, creating its parent directory.
// An empty result means no usable location; the caller falls back to a no-op
// logger. See the package doc for the precedence.
func ResolvePath(tool string) string {
	if tool == "" {
		return ""
	}
	if v := os.Getenv(envName(tool, "LOG_FILE")); v != "" {
		return v
	}
	name := tool + ".log"
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return ensureDir(filepath.Join(xdg, tool, name))
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	if p := ensureDir(filepath.Join(home, ".local", "state", tool, name)); p != "" {
		return p
	}
	return ensureDir(filepath.Join(home, ".cache", tool, name))
}

// StateDir is where a tool keeps non-log state and captured output, following
// the same precedence as ResolvePath. Empty means no usable location.
func StateDir(tool string) string {
	if tool == "" {
		return ""
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, tool)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "state", tool)
}

func ensureDir(p string) string {
	if err := os.MkdirAll(filepath.Dir(p), dirPerm); err != nil {
		return ""
	}
	return p
}

// envName renders FLEET_LOG_FILE from ("fleet", "LOG_FILE"). Hyphens become
// underscores so a tool like tmux-mgr yields a legal variable name.
func envName(tool, suffix string) string {
	t := strings.ToUpper(strings.ReplaceAll(tool, "-", "_"))
	return t + "_" + suffix
}

func parseLevel(tool, s string) logrus.Level {
	if s == "" && tool != "" {
		s = os.Getenv(envName(tool, "LOG_LEVEL"))
	}
	if lv, err := logrus.ParseLevel(s); err == nil {
		return lv
	}
	return logrus.InfoLevel
}

func nonZero(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

var (
	defaultOnce sync.Once
	defaultLog  *logrus.Logger
	defaultTool string
)

// SetDefaultTool names the component the lazy Default() logger belongs to. A
// tool calls this once at startup, before anything logs.
func SetDefaultTool(tool string) { defaultTool = tool }

// Default is the process-wide logger, built lazily from the environment.
// Safe to call concurrently and from any package.
func Default() *logrus.Logger {
	defaultOnce.Do(func() { defaultLog = New(Options{Tool: defaultTool}) })
	return defaultLog
}

// ResetDefaultForTest rearms the lazy singleton so a test can set env and
// observe a fresh logger. Production code MUST NOT call this.
func ResetDefaultForTest() {
	defaultOnce = sync.Once{}
	defaultLog = nil
}
