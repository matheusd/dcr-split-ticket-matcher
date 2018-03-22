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
		ticket     *wire.MsgTx
		splitTx    *wire.MsgTx
		revocation *wire.MsgTx
		err        error
	}

	fundTicketResponse struct {
		ticket     *wire.MsgTx
		revocation *wire.MsgTx
		err        error
	}

	fundSplitTxResponse struct {
		splitTx *wire.MsgTx
		err     error
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
		splitTxOutput    *wire.TxOut
		splitTxChange    *wire.TxOut
		splitTxOutPoints []*wire.OutPoint
		voteAddress      *dcrutil.Address
		poolAddress      *dcrutil.Address
		resp             chan setParticipantOutputsResponse
	}

	fundTicketRequest struct {
		sessionID            SessionID
		ticketInputScriptSig []byte
		revocationScriptSig  []byte
		resp                 chan fundTicketResponse
	}

	fundSplitTxRequest struct {
		sessionID       SessionID
		inputScriptSigs [][]byte
		resp            chan fundSplitTxResponse
	}
)
