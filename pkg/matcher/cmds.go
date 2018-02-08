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

	publishTicketResponse struct {
		tx  *wire.MsgTx
		err error
	}
)

type (
	addParticipantRequest struct {
		maxAmount uint64
		resp      chan addParticipantResponse
	}

	setParticipantOutputsRequest struct {
		sessionID        SessionID
		commitmentOutput *wire.TxOut
		changeOutput     *wire.TxOut
		voteAddress      *dcrutil.Address
		resp             chan setParticipantOutputsResponse
	}

	publishTicketRequest struct {
		sessionID          SessionID
		splitTx            *wire.MsgTx
		input              *wire.TxIn
		splitTxOutputIndex int
		resp               chan publishTicketResponse
	}

	publishSessionRequest struct {
		session *Session
	}
)
