package matcher

import (
	"context"

	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"
)

type (
	addParticipantResponse struct {
		participant *SessionParticipant
		err         error
	}

	setParticipantOutputsResponse struct {
		ticket       *wire.MsgTx
		splitTx      *wire.MsgTx
		participants []*ParticipantTicketOutput
		index        uint32
		err          error
	}

	fundTicketResponse struct {
		tickets     [][]byte
		revocations [][]byte
		err         error
	}

	fundSplitTxResponse struct {
		splitTx []byte
		secrets splitticket.SecretNumbers
		err     error
	}

	publishTicketResponse struct {
		tx  *wire.MsgTx
		err error
	}
)

type (
	addParticipantRequest struct {
		ctx         context.Context
		maxAmount   uint64
		sessionName string
		resp        chan addParticipantResponse
	}

	setParticipantOutputsRequest struct {
		ctx              context.Context
		voteAddress      dcrutil.Address
		poolAddress      dcrutil.Address
		commitAddress    dcrutil.Address
		splitTxAddress   dcrutil.Address
		splitTxOutPoints []*wire.OutPoint
		secretHash       splitticket.SecretNumberHash
		sessionID        ParticipantID
		splitTxChange    *wire.TxOut
		resp             chan setParticipantOutputsResponse
	}

	fundTicketRequest struct {
		ctx                   context.Context
		sessionID             ParticipantID
		ticketsInputScriptSig [][]byte
		revocationScriptSig   []byte
		resp                  chan fundTicketResponse
	}

	fundSplitTxRequest struct {
		ctx             context.Context
		sessionID       ParticipantID
		inputScriptSigs [][]byte
		secretNb        splitticket.SecretNumber
		resp            chan fundSplitTxResponse
	}

	watchWaitingListRequest struct {
		ctx     context.Context
		watcher chan []WaitingQueue
	}
)

type addParticipantRequestsByAmount []*addParticipantRequest

func (a addParticipantRequestsByAmount) Len() int           { return len(a) }
func (a addParticipantRequestsByAmount) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a addParticipantRequestsByAmount) Less(i, j int) bool { return a[i].maxAmount < a[j].maxAmount }
