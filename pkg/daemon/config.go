package daemon

import (
	"github.com/op/go-logging"
)

// Config stores the config needed to run an instance of the dcr split ticket
// matcher daemon
type Config struct {
	Port     int
	LogLevel logging.Level
}

// DefaultConfig stores the default, built-in config for the daemon
var DefaultConfig = &Config{
	Port:     8475,
	LogLevel: logging.INFO,
}
