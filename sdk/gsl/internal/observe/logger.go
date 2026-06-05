package observe

import (
	"io"
	"os"
	"sync"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Options configures a logger built by New. Zero values pick sensible
// defaults — callers can pass Options{} and get a working logger.
type Options struct {
	// Path is the log file location. Empty means "resolve from env via
	// ResolveLogPath"; an empty resolution leaves the logger writing to
	// io.Discard so logging is always safe to call.
	Path string
	// Level is the logrus level ("debug", "info", "warn", "error").
	// Empty falls back to $GSL_LOG_LEVEL, then to "info" on any failure.
	Level string
	// MaxSizeMB is the lumberjack rotation size cap. Zero → 5.
	MaxSizeMB int
	// MaxBackups is the rotated-file retention count. Zero → 3.
	MaxBackups int
	// MaxAgeDays is the rotated-file retention age. Zero → 7.
	MaxAgeDays int
}

// New builds a logger from opts. The returned logger is never nil and
// never panics on use; a logger whose file cannot be opened writes to
// io.Discard. This guarantee lets gsl's hot path call observe freely.
func New(opts Options) *logrus.Logger {
	l := logrus.New()
	l.SetFormatter(&logrus.JSONFormatter{TimestampFormat: "2006-01-02T15:04:05.000Z07:00"})
	l.SetLevel(parseLevel(opts.Level))

	path := opts.Path
	if path == "" {
		path = ResolveLogPath()
	}
	if path == "" {
		l.SetOutput(io.Discard)
		return l
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
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

func parseLevel(s string) logrus.Level {
	if s == "" {
		s = os.Getenv("GSL_LOG_LEVEL")
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
)

// Default returns the process-wide logger, lazily initialized from env
// via New(Options{}). Safe to call concurrently and from any package.
func Default() *logrus.Logger {
	defaultOnce.Do(func() {
		defaultLog = New(Options{})
	})
	return defaultLog
}

// ResetDefaultForTest rearms the lazy singleton so a test can configure
// GSL_LOG_FILE / GSL_LOG_LEVEL via t.Setenv and observe a fresh logger.
// Production code MUST NOT call this.
func ResetDefaultForTest() {
	defaultOnce = sync.Once{}
	defaultLog = nil
}
