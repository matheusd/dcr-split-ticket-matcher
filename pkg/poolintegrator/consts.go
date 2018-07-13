package poolintegrator

import (
	"github.com/decred/dcrd/dcrutil"
	"github.com/pkg/errors"
)

const (
	// DefaultPort that the pool integrator runs on
	DefaultPort = 9872
)

var (
	// ErrHelpRequested is an error returned when the command line options
	// requested the help information
	ErrHelpRequested = errors.New("help requested")

	defaultDataDir = dcrutil.AppDataDir("stmvotepoolintegrator", false)
)
