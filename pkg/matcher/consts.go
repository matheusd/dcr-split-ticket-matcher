package matcher

import "github.com/pkg/errors"

const (
	// MaximumExpiry accepted for split and ticket transactions
	MaximumExpiry = 16
)

type contextKey string

var (
	// EmptySStxChangeAddr is a pre-calculated pkscript for use in SStx change
	// outputs that pays to a zeroed address. This is usually used in change
	// addresses that have zero value.
	EmptySStxChangeAddr = []byte{
		0xbd, 0x76, 0xa9, 0x14, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x88, 0xac}

	originalSrcCtxKey = contextKey("OriginalSrcCtxKey")

	// ErrSessionExpired is the error triggered when the session has expired the
	// maximum allowed elapsed time.
	ErrSessionExpired = errors.New("session expired")
)
