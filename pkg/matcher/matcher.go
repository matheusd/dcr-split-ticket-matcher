package matcher

import (
	"math"

	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/util"
	logging "github.com/op/go-logging"
)

// SessionParticipant is a participant of a split in a given session
type SessionParticipant struct {
	ID                 SessionID
	Amount             dcrutil.Amount
	Fee                dcrutil.Amount
	CommitmentTxOut    *wire.TxOut
	ChangeTxOut        *wire.TxOut
	SplitTxOut         *wire.TxOut
	SplitTxChange      *wire.TxOut
	SplitTxInputs      []*wire.TxIn
	TicketScriptSig    []byte
	SplitTxOutputIndex int
	Session            *Session
	Index              int

	chanSetOutputsResponse  chan setParticipantOutputsResponse
	chanFundTicketResponse  chan fundTicketResponse
	chanFundSplitTxResponse chan fundSplitTxResponse
}

// SessionID stores the unique id for an in-progress ticket buying session
type SessionID int32

// Session is a particular ticket being built
type Session struct {
	Participants []*SessionParticipant
	TicketPrice  dcrutil.Amount
	VoteAddress  dcrutil.Address
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

// CreateTransactions creates the ticket and split tx transactions with all the
// currently available information
func (sess *Session) CreateTransactions() (*wire.MsgTx, *wire.MsgTx, error) {
	ticket := wire.NewMsgTx()
	splitTx := wire.NewMsgTx()

	if sess.VoteAddress == nil {
		return nil, nil, ErrNoVoteAddress
	}

	script, err := txscript.PayToSStx(sess.VoteAddress)
	if err != nil {
		return nil, nil, err
	}
	txout := wire.NewTxOut(int64(sess.TicketPrice), script)
	ticket.AddTxOut(txout)

	var spOutIndex uint32

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

	return ticket, splitTx, nil
}

type TicketPriceProvider interface {
	CurrentTicketPrice() uint64
}

type VoteAddressProvider interface {
	VotingAddress() dcrutil.Address
	SignRevocation(ticket, revocation *wire.MsgTx) (*wire.MsgTx, error)
}

// Config stores the parameters for the matcher engine
type Config struct {
	MinAmount             uint64
	MaxOnlineParticipants int
	PriceProvider         TicketPriceProvider
	VoteAddrProvider      VoteAddressProvider
	LogLevel              logging.Level
}

// Matcher is the main engine for matching operations
type Matcher struct {
	waitingParticipants []*addParticipantRequest
	sessions            map[SessionID]*SessionParticipant
	cfg                 *Config
	log                 *logging.Logger

	addParticipantRequests        chan addParticipantRequest
	setParticipantOutputsRequests chan setParticipantOutputsRequest
	fundTicketRequests            chan fundTicketRequest
	fundSplitTxRequests           chan fundSplitTxRequest
}

func NewMatcher(cfg *Config) *Matcher {
	m := &Matcher{
		cfg:                           cfg,
		log:                           logging.MustGetLogger("matcher"),
		sessions:                      make(map[SessionID]*SessionParticipant),
		addParticipantRequests:        make(chan addParticipantRequest),
		setParticipantOutputsRequests: make(chan setParticipantOutputsRequest),
		fundTicketRequests:            make(chan fundTicketRequest),
		fundSplitTxRequests:           make(chan fundSplitTxRequest),
	}
	util.SetLoggerBackend(true, "", "", cfg.LogLevel, m.log)

	m.addParticipantRequests = make(chan addParticipantRequest, cfg.MaxOnlineParticipants)

	return m
}

// Run listens for all matcher messages and runs the matching engine.
func (matcher *Matcher) Run() error {
	for {
		select {
		case req := <-matcher.addParticipantRequests:
			matcher.log.Infof("Adding participant for amount %s", dcrutil.Amount(req.maxAmount))
			matcher.waitingParticipants = append(matcher.waitingParticipants, &req)
			if matcher.enoughForNewSession() {
				matcher.startNewSession()
			}
		case req := <-matcher.setParticipantOutputsRequests:
			err := matcher.setParticipantsOutputs(&req)
			if err != nil {
				req.resp <- setParticipantOutputsResponse{
					err: err,
				}
			}
		case req := <-matcher.fundTicketRequests:
			err := matcher.fundTicket(&req)
			if err != nil {
				req.resp <- fundTicketResponse{
					err: err,
				}
			}
		case req := <-matcher.fundSplitTxRequests:
			err := matcher.fundSplitTx(&req)
			if err != nil {
				req.resp <- fundSplitTxResponse{
					err: err,
				}
			}
		}
	}
}

func (matcher *Matcher) enoughForNewSession() bool {

	ticketPrice := dcrutil.Amount(matcher.cfg.PriceProvider.CurrentTicketPrice())
	var availableSum uint64

	for _, r := range matcher.waitingParticipants {
		availableSum += r.maxAmount
	}

	ticketFee := SessionFeeEstimate(len(matcher.waitingParticipants))
	neededAmount := uint64(ticketPrice + ticketFee)
	return availableSum > neededAmount
}

func (matcher *Matcher) startNewSession() {
	numParts := len(matcher.waitingParticipants)
	ticketFee := SessionFeeEstimate(numParts)
	partFee := dcrutil.Amount(math.Ceil(float64(ticketFee) / float64(numParts)))

	matcher.log.Infof("Starting new session: Ticket Price=%s Fees=%s Participants=%d",
		dcrutil.Amount(matcher.cfg.PriceProvider.CurrentTicketPrice()),
		ticketFee, numParts)

	sess := &Session{
		Participants: make([]*SessionParticipant, numParts),
		TicketPrice:  dcrutil.Amount(matcher.cfg.PriceProvider.CurrentTicketPrice()),
		VoteAddress:  matcher.cfg.VoteAddrProvider.VotingAddress(),
	}

	amountLeft := dcrutil.Amount(matcher.cfg.PriceProvider.CurrentTicketPrice())
	for i, r := range matcher.waitingParticipants {
		amount := dcrutil.Amount(r.maxAmount) - partFee
		if amount > amountLeft {
			amount = amountLeft
		}

		sessPart := &SessionParticipant{
			Amount:  amount,
			Fee:     partFee,
			Session: sess,
			Index:   i,
		}
		sess.Participants[i] = sessPart

		id := matcher.newSessionID()
		matcher.sessions[id] = sessPart
		sessPart.ID = id

		r.resp <- addParticipantResponse{
			participant: sessPart,
		}

		amountLeft -= amount
	}

	matcher.waitingParticipants = nil
}

func (matcher *Matcher) newSessionID() SessionID {
	// TODO: rw lock matcher.sessions here
	id := MustRandInt32()
	for _, has := matcher.sessions[SessionID(id)]; has; {
		id = MustRandInt32()
	}
	return SessionID(id)
}

func (matcher *Matcher) setParticipantsOutputs(req *setParticipantOutputsRequest) error {

	revocationFee, err := dcrutil.NewAmount(0.001) // parametrize
	if err != nil {
		panic(err)
	}

	if _, has := matcher.sessions[req.sessionID]; !has {
		return ErrSessionNotFound.WithMessagef("Session with ID %d not found", req.sessionID)
	}

	if req.commitmentOutput == nil {
		return ErrNilCommitmentOutput
	}

	if req.changeOutput == nil {
		return ErrNilChangeOutput
	}

	sess := matcher.sessions[req.sessionID]
	// TODO: decode and check commitment amount
	// if req.commitmentOutput.Value != int64(sess.Amount) {
	// 	return ErrCommitmentValueDifferent.
	// 		WithMessagef("Commitment amount %s different then expected %s", dcrutil.Amount(req.commitmentOutput.Value), sess.Amount).
	// 		WithValue("expectedAmount", sess.Amount).
	// 		WithValue("providedAmount", req.commitmentOutput.Value)
	// }
	if req.splitTxOutput.Value != int64(sess.Amount+sess.Fee) {
		return ErrSplitValueInputValueMismatch.
			WithMessagef("Split txOut.value (%s) !== session.amount (%s) + session.fee(%s)", dcrutil.Amount(req.splitTxOutput.Value), sess.Amount, sess.Fee)
	}

	if len(req.splitTxOutPoints) == 0 {
		return ErrNoSplitTxInputOutPoints
	}

	// TODO: get the utxos from network and validate whether the sum(utxos) == splitTxOut + splitTxChange + SplitTxFee

	matcher.log.Infof("Participant %d set output commitment %s", req.sessionID, sess.Amount)

	sess.CommitmentTxOut = req.commitmentOutput
	sess.ChangeTxOut = req.changeOutput
	sess.chanSetOutputsResponse = req.resp
	sess.SplitTxOut = req.splitTxOutput
	sess.SplitTxChange = req.splitTxChange
	sess.SplitTxInputs = make([]*wire.TxIn, len(req.splitTxOutPoints))
	for i, outp := range req.splitTxOutPoints {
		sess.SplitTxInputs[i] = wire.NewTxIn(outp, nil)
	}

	if sess.Session.AllOutputsFilled() {
		matcher.log.Infof("All outputs for session received. Creating txs.")

		var ticket, splitTx, unsignedRevoke, revocation *wire.MsgTx
		var err error

		ticket, splitTx, err = sess.Session.CreateTransactions()
		if err == nil {
			ticketHash := ticket.TxHash()
			unsignedRevoke, err = createUnsignedRevocation(&ticketHash, ticket, revocationFee)
			if err == nil {
				revocation, err = matcher.cfg.VoteAddrProvider.SignRevocation(ticket, unsignedRevoke)
				if err != nil {
					matcher.log.Errorf("Error signing revocation: %v", err)
				}
			} else {
				matcher.log.Errorf("Error generating revocation: %v", err)
			}
		} else {
			matcher.log.Errorf("Error generating session transactions: %v", err)
		}

		for _, p := range sess.Session.Participants {
			p.chanSetOutputsResponse <- setParticipantOutputsResponse{
				ticket:     ticket,
				splitTx:    splitTx,
				revocation: revocation,
				err:        err,
			}
		}
	}

	return nil
}

func (matcher *Matcher) fundTicket(req *fundTicketRequest) error {
	if _, has := matcher.sessions[req.sessionID]; !has {
		return ErrSessionNotFound
	}

	sess := matcher.sessions[req.sessionID]
	if sess.ChangeTxOut == nil {
		return ErrNilChangeOutput
	}

	if sess.CommitmentTxOut == nil {
		return ErrNilCommitmentOutput
	}

	// TODO: check if ticketInputScriptSig actually commits to the ticket's outputs
	// and is of the correct sigHashType

	sess.TicketScriptSig = req.ticketInputScriptSig
	sess.chanFundTicketResponse = req.resp

	if sess.Session.TicketIsFunded() {
		matcher.log.Infof("All sigscripts for ticket received. Creating funded ticket.")

		ticket, _, err := sess.Session.CreateTransactions()

		for _, p := range sess.Session.Participants {
			p.chanFundTicketResponse <- fundTicketResponse{
				ticket: ticket,
				err:    err,
			}
			matcher.log.Infof("Alerted participant %d of funded ticked", p.ID)
		}
	}

	return nil
}

func (matcher *Matcher) fundSplitTx(req *fundSplitTxRequest) error {
	if _, has := matcher.sessions[req.sessionID]; !has {
		return ErrSessionNotFound
	}
	sess := matcher.sessions[req.sessionID]

	if len(sess.SplitTxInputs) != len(req.inputScriptSigs) {
		return ErrSplitInputSignLenMismatch.
			WithMessagef("len(splitTxInputs %d) != len(inputScriptSigs %d)", len(sess.SplitTxInputs), len(req.inputScriptSigs))
	}

	// TODO: make sure the script sigs actually sign the split tx

	for i, script := range req.inputScriptSigs {
		sess.SplitTxInputs[i].SignatureScript = script
	}
	sess.chanFundSplitTxResponse = req.resp

	if sess.Session.SplitTxIsFunded() {
		matcher.log.Infof("All inputs for split tx received. Creating split tx.")

		_, splitTx, err := sess.Session.CreateTransactions()

		for _, p := range sess.Session.Participants {
			p.chanFundSplitTxResponse <- fundSplitTxResponse{
				splitTx: splitTx,
				err:     err,
			}
			matcher.log.Infof("Alerted participant %d of funded split tx", p.ID)
		}
	}

	return nil
}

func (matcher *Matcher) AddParticipant(maxAmount uint64) (*SessionParticipant, error) {
	if maxAmount < matcher.cfg.MinAmount {
		return nil, ErrLowAmount
	}

	// TODO: not really great. Should be automatically calc'd by the ticket
	// fee + minimum amount for each participant (which is determined by
	// ticket fee + max amount of commitments in an SSTX)
	if len(matcher.waitingParticipants) >= matcher.cfg.MaxOnlineParticipants {
		return nil, ErrTooManyParticipants
	}

	req := addParticipantRequest{
		maxAmount: maxAmount,
		resp:      make(chan addParticipantResponse),
	}
	matcher.addParticipantRequests <- req

	resp := <-req.resp
	return resp.participant, resp.err
}

// SetParticipantsOutputs validates and sets the outputs of the given participant
// for the provided outputs, waits for all participants to provide their own
// outputs, then generates the ticket tx and returns the index of the input
// that should receive this participants funds
func (matcher *Matcher) SetParticipantsOutputs(sessionID SessionID, commitmentOutput,
	changeOutput wire.TxOut, voteAddress dcrutil.Address, splitTxChange wire.TxOut,
	splitTxOutput wire.TxOut, splitTxOutPoints []*wire.OutPoint) (*wire.MsgTx, *wire.MsgTx, *wire.MsgTx, error) {

	outpoints := make([]*wire.OutPoint, len(splitTxOutPoints))
	for i, o := range splitTxOutPoints {
		outpoints[i] = o
	}

	req := setParticipantOutputsRequest{
		sessionID:        sessionID,
		commitmentOutput: &commitmentOutput,
		changeOutput:     &changeOutput,
		voteAddress:      &voteAddress,
		splitTxChange:    &splitTxChange,
		splitTxOutput:    &splitTxOutput,
		splitTxOutPoints: outpoints,
		resp:             make(chan setParticipantOutputsResponse),
	}

	matcher.setParticipantOutputsRequests <- req
	resp := <-req.resp
	return resp.ticket, resp.splitTx, resp.revocation, resp.err
}

func (matcher *Matcher) FundTicket(sessionID SessionID, inputScriptSig []byte) (*wire.MsgTx, error) {

	req := fundTicketRequest{
		sessionID:            sessionID,
		ticketInputScriptSig: inputScriptSig,
		resp:                 make(chan fundTicketResponse),
	}

	matcher.fundTicketRequests <- req
	resp := <-req.resp
	return resp.ticket, resp.err
}

func (matcher *Matcher) FundSplit(sessionID SessionID, inputScriptSigs [][]byte) (*wire.MsgTx, error) {

	req := fundSplitTxRequest{
		sessionID:       sessionID,
		inputScriptSigs: inputScriptSigs,
		resp:            make(chan fundSplitTxResponse),
	}
	matcher.fundSplitTxRequests <- req
	resp := <-req.resp
	return resp.splitTx, resp.err

}
