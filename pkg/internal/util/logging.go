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
