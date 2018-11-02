package util

import (
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/decred/slog"
)

// LogFileName returns a new non-existant log filename that can be used as a new
// log file in the given dir.
func LogFileName(dir string, baseName string) string {
	dateNow := time.Now().Format("2006-01-02")
	timeNow := time.Now().Format("150405")

	baseName = strings.Replace(baseName, "{date}", dateNow, -1)
	baseName = strings.Replace(baseName, "{time}", timeNow, -1)

	i := 0
	fname := path.Join(dir, baseName)
	for _, err := os.Stat(fname); (err != nil) && !os.IsNotExist(err); i++ {
		fname = path.Join(dir, fmt.Sprintf(baseName+"-%3d", i))
	}

	return fname
}

// logFileBackend returns a backend configured to write to a log file
// in the given dir, using the given baseName
func logFileBackend(dir string, baseName string) io.Writer {
	fname := LogFileName(dir, baseName)
	f, err := os.OpenFile(fname, os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		panic(err)
	}

	return f
}

type multiWriter []io.Writer

func (w multiWriter) Write(b []byte) (int, error) {
	for i := range w {
		n, err := w[i].Write(b)
		if err != nil {
			return n, err
		}
	}
	return len(b), nil
}

// StandardLogBackend returns a standard backend that can output to stderr and
// to a file
func StandardLogBackend(toStdErr bool, dir string, baseName string) *slog.Backend {
	multi := make(multiWriter, 0, 2)

	if toStdErr {
		multi = append(multi, os.Stderr)
	}

	if dir != "" {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			os.MkdirAll(dir, 0755)
		}

		fileBackend := logFileBackend(dir, baseName)
		multi = append(multi, fileBackend)
	}

	return slog.NewBackend(multi)
}

// PrefixLogger logs messages prefixed with a given string prefix
type PrefixLogger struct {
	parent        slog.Logger
	prefix        string
	prefixNoSpace string
}

// NewPrefixLogger returns a new PrefixLogger configured to use the given prefix
// and send log messages to the specified parent
func NewPrefixLogger(prefix string, parent slog.Logger) *PrefixLogger {
	prefix = strings.TrimSpace(prefix)
	prefixNoSpace := prefix
	if prefix != "" {
		prefix += " "
	}

	return &PrefixLogger{
		prefix:        prefix,
		prefixNoSpace: prefixNoSpace,
		parent:        parent,
	}
}

func (pl *PrefixLogger) unshiftPrefix(v ...interface{}) []interface{} {
	if pl.prefix != "" {
		newv := make([]interface{}, 0, len(v)+1)
		newv = append(newv, pl.prefixNoSpace)
		newv = append(newv, v...)
		return newv
	}

	return v
}

// Tracef fulfills the slog.Logger interface
func (pl *PrefixLogger) Tracef(format string, params ...interface{}) {
	pl.parent.Tracef(pl.prefix+format, params...)
}

// Debugf fulfills the slog.Logger interface
func (pl *PrefixLogger) Debugf(format string, params ...interface{}) {
	pl.parent.Debugf(pl.prefix+format, params...)
}

// Infof fulfills the slog.Logger interface
func (pl *PrefixLogger) Infof(format string, params ...interface{}) {
	pl.parent.Infof(pl.prefix+format, params...)
}

// Warnf fulfills the slog.Logger interface
func (pl *PrefixLogger) Warnf(format string, params ...interface{}) {
	pl.parent.Warnf(pl.prefix+format, params...)
}

// Errorf fulfills the slog.Logger interface
func (pl *PrefixLogger) Errorf(format string, params ...interface{}) {
	pl.parent.Errorf(pl.prefix+format, params...)
}

// Criticalf fulfills the slog.Logger interface
func (pl *PrefixLogger) Criticalf(format string, params ...interface{}) {
	pl.parent.Criticalf(pl.prefix+format, params...)
}

// Debug fulfills the slog.Logger interface
func (pl *PrefixLogger) Debug(v ...interface{}) { pl.parent.Debug(pl.unshiftPrefix(v...)...) }

// Trace fulfills the slog.Logger interface
func (pl *PrefixLogger) Trace(v ...interface{}) { pl.parent.Trace(pl.unshiftPrefix(v...)...) }

// Info fulfills the slog.Logger interface
func (pl *PrefixLogger) Info(v ...interface{}) { pl.parent.Info(pl.unshiftPrefix(v...)...) }

// Warn fulfills the slog.Logger interface
func (pl *PrefixLogger) Warn(v ...interface{}) { pl.parent.Warn(pl.unshiftPrefix(v...)...) }

// Error fulfills the slog.Logger interface
func (pl *PrefixLogger) Error(v ...interface{}) { pl.parent.Error(pl.unshiftPrefix(v...)...) }

// Critical fulfills the slog.Logger interface
func (pl *PrefixLogger) Critical(v ...interface{}) { pl.parent.Critical(pl.unshiftPrefix(v...)...) }

// Level fulfills the slog.Logger interface
func (pl *PrefixLogger) Level() slog.Level { return pl.parent.Level() }

// SetLevel fulfills the slog.Logger interface
func (pl *PrefixLogger) SetLevel(level slog.Level) { pl.parent.SetLevel(level) }
