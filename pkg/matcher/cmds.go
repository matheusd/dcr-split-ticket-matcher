package matcher

import (
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
)

type (
	addParticipantResponse struct {
		participant *SessionParticipant
		err         error
	}

	setParticipantOutputsResponse struct {
		transaction  *wire.MsgTx
		output_index int
		err          error
	}
)

type (
	addParticipantRequest struct {
		participant *Participant
		resp        chan addParticipantResponse
	}

	setParticipantOutputsRequest struct {
		sessionID        SessionID
		commitmentOutput *wire.TxOut
		changeOutput     *wire.TxOut
		voteAddress      *dcrutil.Address
		resp             chan setParticipantOutputsResponse
	}
)
