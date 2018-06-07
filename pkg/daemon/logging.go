package daemon

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	logging "github.com/op/go-logging"
)

// coloredLogFormatter is a formatter that outputs strings with color information
// (usefull for debugging on console)
var coloredLogFormatter = logging.MustStringFormatter(
	`%{color}%{time:2006-01-02 15:04:05.000} %{id:03x} %{shortfunc:20s} ▶ %{level:.4s}%{color:reset} %{message}`,
)

// defaultLogFormatter is the default formatter to be used on lablock projects
var defaultLogFormatter = logging.MustStringFormatter(
	`%{time:2006-01-02 15:04:05.000} %{id:03x} %{shortfunc} ▶ %{level:.4s} %{message}`,
)

// logFileName returns a new non-existant log filename that can be used as a new
// log file in the given dir.
func logFileName(dir string, baseName string) string {
	dateNow := time.Now().Format("2006-01-02")
	timeNow := time.Now().Format("150405")

	baseName = strings.Replace(baseName, "{date}", dateNow, -1)
	baseName = strings.Replace(baseName, "{time}", timeNow, -1)

	i := 0
	fname := path.Join(dir, baseName)
	for _, err := os.Stat(fname); (err != nil) && !os.IsNotExist(err); i++ {
		fmt.Println(fname, err)
		fname = path.Join(dir, fmt.Sprintf(baseName+"-%3d", i))
	}

	return fname
}

// logFileBackend returns a backend configured to write to a log file
// in the given dir, using the given baseName
func logFileBackend(dir string, baseName string) logging.Backend {
	fname := logFileName(dir, baseName)
	f, err := os.OpenFile(fname, os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		panic(err)
	}

	backend := logging.NewLogBackend(f, "", 0)
	fmtd := logging.NewBackendFormatter(backend, defaultLogFormatter)
	return fmtd
}

// standardLogBackend returns a standard backend that can output to stderr and
// to a file
func standardLogBackend(toStdErr bool, dir string, baseName string, logLevel logging.Level) logging.LeveledBackend {
	var backends []logging.Backend

	if toStdErr {
		stderrBackend := logging.NewLogBackend(os.Stderr, "", 0)
		stderrBackendFmt := logging.NewBackendFormatter(stderrBackend, coloredLogFormatter)
		stderrBackendLvl := logging.AddModuleLevel(stderrBackendFmt)
		stderrBackendLvl.SetLevel(logLevel, "")
		backends = append(backends, stderrBackendLvl)
	}

	if dir != "" {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			os.MkdirAll(dir, 0755)
		}

		fileBackend := logFileBackend(dir, baseName)
		fileBackendFmt := logging.NewBackendFormatter(fileBackend, defaultLogFormatter)
		fileBackendLvl := logging.AddModuleLevel(fileBackendFmt)
		fileBackendLvl.SetLevel(logLevel, "")
		backends = append(backends, fileBackendLvl)
	}

	return logging.MultiLogger(backends...)
}
