package daemon

import (
	"path/filepath"

	"github.com/decred/dcrd/dcrutil"
	"github.com/op/go-logging"
)

// Config stores the config needed to run an instance of the dcr split ticket
// matcher daemon
type Config struct {
	Port     int
	LogLevel logging.Level
	LogDir   string
	KeyFile  string
	CertFile string
	DcrdHost string
	DcrdUser string
	DcrdPass string
	DcrdCert string
}

// DefaultConfig stores the default, built-in config for the daemon
var DefaultConfig = &Config{
	Port:     8475,
	LogLevel: logging.INFO,
	LogDir:   "./data/logs",
	KeyFile:  "./data/rpc.key",
	CertFile: "./data/rpc.cert",
	DcrdHost: "localhost:19119",
	DcrdUser: "USER",
	DcrdPass: "PASSWORD",
	DcrdCert: filepath.Join(dcrutil.AppDataDir("dcrd", false), "rpc.cert"),
}
