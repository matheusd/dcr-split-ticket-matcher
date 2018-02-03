package daemon

import (
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/util"
	"github.com/op/go-logging"
)

// Daemon is the main instance of a running dcr split ticket matcher daemon
type Daemon struct {
	cfg *Config
	log *logging.Logger
}

// NewDaemon returns a new daemon instance and prepares it to listen to
// requests.
func NewDaemon(cfg *Config) (*Daemon, error) {
	d := &Daemon{
		cfg: cfg,
		log: logging.MustGetLogger("dcr-split-ticket-matcher"),
	}

	util.SetLoggerBackend(true, "", "", cfg.LogLevel, d.log)

	return d, nil
}

// ListenAndServe connections for the daemon. Returns an error when done.
func (daemon *Daemon) ListenAndServe() error {
	daemon.log.Info("Ready to listen")
	return nil
}
