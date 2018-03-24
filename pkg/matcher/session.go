package matcher

import (
	"fmt"
	"time"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

// ParticipantID is the unique ID of a participant of a session
type ParticipantID uint32

func (id ParticipantID) String() string {
	upper := uint32(id) >> 16
	lower := uint32(id) & 0xffff
	return fmt.Sprintf("%.4x.%.4x", upper, lower)
}

// SessionParticipant is a participant of a split in a given session
type SessionParticipant struct {
	ID                  ParticipantID
	CommitAmount        dcrutil.Amount
	Fee                 dcrutil.Amount
	PoolFee             dcrutil.Amount
	CommitmentTxOut     *wire.TxOut
	ChangeTxOut         *wire.TxOut
	SplitTxOut          *wire.TxOut
	SplitTxChange       *wire.TxOut
	SplitTxInputs       []*wire.TxIn
	TicketScriptSig     []byte
	RevocationScriptSig []byte
	SplitTxOutputIndex  int
	Session             *Session
	VoteAddress         *dcrutil.Address
	PoolAddress         *dcrutil.Address
	Index               int

	chanSetOutputsResponse  chan setParticipantOutputsResponse
	chanFundTicketResponse  chan fundTicketResponse
	chanFundSplitTxResponse chan fundSplitTxResponse
}

func (part *SessionParticipant) sendSetOutputsResponse(resp setParticipantOutputsResponse) {
	c := part.chanSetOutputsResponse
	part.chanSetOutputsResponse = nil
	c <- resp
}

func (part *SessionParticipant) sendFundTicketResponse(resp fundTicketResponse) {
	c := part.chanFundTicketResponse
	part.chanFundTicketResponse = nil
	c <- resp
}

func (part *SessionParticipant) sendFundSplitTxResponse(resp fundSplitTxResponse) {
	c := part.chanFundSplitTxResponse
	part.chanFundSplitTxResponse = nil
	c <- resp
}

func (part *SessionParticipant) sessionCanceled(err error) {
	if part.chanSetOutputsResponse != nil {
		part.sendSetOutputsResponse(setParticipantOutputsResponse{err: err})
	}
	if part.chanFundTicketResponse != nil {
		part.sendFundTicketResponse(fundTicketResponse{err: err})
	}
	if part.chanFundSplitTxResponse != nil {
		part.sendFundSplitTxResponse(fundSplitTxResponse{err: err})
	}
}

// SessionID stores the unique id for an in-progress ticket buying session
type SessionID uint16

func (id SessionID) String() string {
	return fmt.Sprintf("%.4x", uint16(id))
}

// Session is a particular ticket being built
type Session struct {
	ID             SessionID
	Participants   []*SessionParticipant
	TicketPrice    dcrutil.Amount
	VoterIndex     int
	PoolFee        dcrutil.Amount
	ChainParams    *chaincfg.Params
	SplitTxPoolOut *wire.TxOut
	TicketPoolIn   *wire.TxIn
	StartTime      time.Time
	Done           bool
	Canceled       bool
}

// AllOutputsFilled returns true if all commitment and change outputs for all
// participants have been filled
func (sess *Session) AllOutputsFilled() bool {
	for _, p := range sess.Participants {
		if p.ChangeTxOut == nil || p.CommitmentTxOut == nil || p.SplitTxOut == nil ||
			len(p.SplitTxInputs) == 0 {

			return false
		}
	}
	return true
}

// TicketIsFunded returns true if all ticket inputs have been filled.
func (sess *Session) TicketIsFunded() bool {
	for _, p := range sess.Participants {
		if p.TicketScriptSig == nil {
			return false
		}
	}
	return true
}

// SplitTxIsFunded returns true if the split tx is funded
func (sess *Session) SplitTxIsFunded() bool {
	for _, p := range sess.Participants {
		for _, in := range p.SplitTxInputs {
			if len(in.SignatureScript) == 0 {
				return false
			}
		}
	}
	return true
}

func (sess *Session) addCommonTicketOutputs(ticket *wire.MsgTx) error {
	voter := sess.Participants[sess.VoterIndex]

	if voter.VoteAddress == nil {
		return ErrNoVoteAddress
	}

	if voter.PoolAddress == nil {
		return ErrNoPoolAddress
	}

	voteScript, err := txscript.PayToSStx(*voter.VoteAddress)
	if err != nil {
		return err
	}

	limits := uint16(0x5800)
	commitScript, err := txscript.GenerateSStxAddrPush(*voter.PoolAddress,
		sess.PoolFee, limits)
	if err != nil {
		return err
	}

	zeroed := [20]byte{}
	addrZeroed, err := dcrutil.NewAddressPubKeyHash(zeroed[:], sess.ChainParams, 0)
	if err != nil {
		return err
	}
	changeScript, err := txscript.PayToSStxChange(addrZeroed)
	if err != nil {
		return err
	}

	ticket.AddTxIn(sess.TicketPoolIn)
	ticket.AddTxOut(wire.NewTxOut(int64(sess.TicketPrice), voteScript))
	ticket.AddTxOut(wire.NewTxOut(0, commitScript))
	ticket.AddTxOut(wire.NewTxOut(0, changeScript))

	return nil
}

// CreateTransactions creates the ticket and split tx transactions with all the
// currently available information
func (sess *Session) CreateTransactions() (*wire.MsgTx, *wire.MsgTx, *wire.MsgTx, error) {
	var spOutIndex uint32 = 0
	var err error

	ticket := wire.NewMsgTx()
	splitTx := wire.NewMsgTx()
	revocation := wire.NewMsgTx()

	sess.addCommonTicketOutputs(ticket)

	splitTx.AddTxOut(sess.SplitTxPoolOut)
	spOutIndex++
	for _, p := range sess.Participants {
		if p.SplitTxOut != nil {
			ticket.AddTxIn(wire.NewTxIn(&wire.OutPoint{Index: spOutIndex}, p.TicketScriptSig))
			splitTx.AddTxOut(p.SplitTxOut)
			spOutIndex++
		}

		if p.SplitTxChange != nil {
			splitTx.AddTxOut(p.SplitTxChange)
			spOutIndex++
		}

		if p.CommitmentTxOut != nil {
			ticket.AddTxOut(p.CommitmentTxOut)
		}

		if p.ChangeTxOut != nil {
			ticket.AddTxOut(p.ChangeTxOut)
		}

		for _, in := range p.SplitTxInputs {
			splitTx.AddTxIn(in)
		}
	}

	// back-fill the ticket input's outpoints with the split tx hash
	splitHash := splitTx.TxHash()
	for _, in := range ticket.TxIn {
		in.PreviousOutPoint.Hash = splitHash
	}

	revocationFee, _ := dcrutil.NewAmount(0.001) // FIXME parametrize

	ticketHash := ticket.TxHash()
	revocation, err = createUnsignedRevocation(&ticketHash, ticket, revocationFee)
	if err != nil {
		return nil, nil, nil, err
	}

	voter := sess.Participants[sess.VoterIndex]
	if voter.RevocationScriptSig != nil {
		revocation.TxIn[0].SignatureScript = voter.RevocationScriptSig
	}

	return ticket, splitTx, revocation, nil
}
