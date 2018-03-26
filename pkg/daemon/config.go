package daemon

import (
	"github.com/op/go-logging"
)

// Config stores the config needed to run an instance of the dcr split ticket
// matcher daemon
type Config struct {
	Port     int
	LogLevel logging.Level
	LogDir   string
	KeyFile string
	CertFile string
}

// DefaultConfig stores the default, built-in config for the daemon
var DefaultConfig = &Config{
	Port:     8475,
	LogLevel: logging.INFO,
	LogDir:   "./data/logs",
	KeyFile: "./data/rpc.key",
	CertFile: "./data/rpc.cert",
}
