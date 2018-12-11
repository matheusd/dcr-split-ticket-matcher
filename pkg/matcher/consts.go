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

// SessionStage is the stage of a given session
type SessionStage int

// String returns the string representation of the session stage
func (ss SessionStage) String() string {
	switch ss {
	case StageUnknown:
		return "unknown"
	case StageWaitingOutputs:
		return "waiting outputs"
	case StageWaitingTicketFunds:
		return "waiting ticket funds"
	case StageWaitingSplitFunds:
		return "waiting split funds"
	case StageDone:
		return "done"
	default:
		return "invalid"
	}
}

// The below constants are for the possible stages a session can be in.
const (
	StageUnknown SessionStage = iota
	StageWaitingOutputs
	StageWaitingTicketFunds
	StageWaitingSplitFunds
	StageDone
)
