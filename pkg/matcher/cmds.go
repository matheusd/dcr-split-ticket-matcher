package matcher

import (
	"context"

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
		ctx       context.Context
		maxAmount uint64
		resp      chan addParticipantResponse
	}

	setParticipantOutputsRequest struct {
		ctx              context.Context
		sessionID        ParticipantID
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
		ctx                  context.Context
		sessionID            ParticipantID
		ticketInputScriptSig []byte
		revocationScriptSig  []byte
		resp                 chan fundTicketResponse
	}

	fundSplitTxRequest struct {
		ctx             context.Context
		sessionID       ParticipantID
		inputScriptSigs [][]byte
		resp            chan fundSplitTxResponse
	}

	watchWaitingListRequest struct {
		ctx     context.Context
		watcher chan []dcrutil.Amount
	}
)

type addParticipantRequestsByAmount []*addParticipantRequest

func (a addParticipantRequestsByAmount) Len() int           { return len(a) }
func (a addParticipantRequestsByAmount) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a addParticipantRequestsByAmount) Less(i, j int) bool { return a[i].maxAmount < a[j].maxAmount }
