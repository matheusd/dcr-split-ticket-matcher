package matcher

import (
	"math"

	"github.com/ansel1/merry"
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
	VoteAddress        *dcrutil.Address
	CommitmentTxOut    *wire.TxOut
	ChangeTxOut        *wire.TxOut
	Input              *wire.TxIn
	SplitTx            *wire.MsgTx
	SplitTxOutputIndex int
	Session            *Session
	Index              int

	chanSetOutputsResponse    chan setParticipantOutputsResponse
	chanPublishTicketResponse chan publishTicketResponse
}

// SessionID stores the unique id for an in-progress ticket buying session
type SessionID int32

// Session is a particular ticket being built
type Session struct {
	Participants []*SessionParticipant
	TicketPrice  dcrutil.Amount
	VoterIndex   int
}

// AllOutputsFilled returns true if all commitment and change outputs for all
// participants have been filled
func (sess *Session) AllOutputsFilled() bool {
	for _, p := range sess.Participants {
		if p.ChangeTxOut == nil || p.CommitmentTxOut == nil {
			return false
		}
	}
	return true
}

// AllInputsFilled returns true if all inputs and split transactions for
// all participants have been filled
func (sess *Session) AllInputsFilled() bool {
	for _, p := range sess.Participants {
		if p.Input == nil || p.SplitTx == nil {
			return false
		}
	}
	return true
}

// CreateTransaction creates the transaction with all the currently available
// information
func (sess *Session) CreateTransaction() (*wire.MsgTx, error) {
	tx := wire.NewMsgTx()

	if sess.Participants[sess.VoterIndex].VoteAddress != nil {
		script, err := txscript.PayToSStx(*sess.Participants[sess.VoterIndex].VoteAddress)
		if err != nil {
			return nil, err
		}
		txout := wire.NewTxOut(int64(sess.TicketPrice), script)
		tx.AddTxOut(txout)
	}

	for _, p := range sess.Participants {
		if p.Input != nil {
			tx.AddTxIn(p.Input)
		}

		if p.CommitmentTxOut != nil {
			tx.AddTxOut(p.CommitmentTxOut)
		}

		if p.ChangeTxOut != nil {
			tx.AddTxOut(p.ChangeTxOut)
		}
	}

	return tx, nil
}

type TicketPriceProvider interface {
	CurrentTicketPrice() uint64
}

// Config stores the parameters for the matcher engine
type Config struct {
	MinAmount             uint64
	MaxOnlineParticipants int
	PriceProvider         TicketPriceProvider
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
	publishTicketRequests         chan publishTicketRequest
}

func NewMatcher(cfg *Config) *Matcher {
	m := &Matcher{
		cfg:                           cfg,
		log:                           logging.MustGetLogger("matcher"),
		sessions:                      make(map[SessionID]*SessionParticipant),
		addParticipantRequests:        make(chan addParticipantRequest),
		setParticipantOutputsRequests: make(chan setParticipantOutputsRequest),
		publishTicketRequests:         make(chan publishTicketRequest),
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
		case req := <-matcher.publishTicketRequests:
			err := matcher.addParticipantInput(&req)
			if err != nil {
				req.resp <- publishTicketResponse{
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
		VoterIndex:   0, // FIXME: select voter index at random
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

	matcher.log.Infof("Participant %d set output commitment %s", req.sessionID, sess.Amount)

	sess.CommitmentTxOut = req.commitmentOutput
	sess.ChangeTxOut = req.changeOutput
	sess.chanSetOutputsResponse = req.resp
	sess.VoteAddress = req.voteAddress

	if sess.Session.AllOutputsFilled() {
		matcher.log.Infof("All outputs for session received. Creating tx.")
		tx, err := sess.Session.CreateTransaction()
		for _, p := range sess.Session.Participants {
			p.chanSetOutputsResponse <- setParticipantOutputsResponse{
				output_index: p.Index,
				transaction:  tx,
				err:          err,
			}
		}
	}

	return nil
}

func (matcher *Matcher) addParticipantInput(req *publishTicketRequest) error {

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

	if req.splitTxOutputIndex >= len(req.splitTx.TxOut) {
		return ErrIndexNotFound.WithValue("index", req.splitTxOutputIndex)
	}

	splitTxOut := req.splitTx.TxOut[req.splitTxOutputIndex]
	// if splitTxOut.Value != req.input.ValueIn {
	// 	return ErrSplitValueInputValueMismatch.
	// 		WithMessagef("Split txOut.value (%d) !== input.ValueIn (%d)", splitTxOut.Value, req.input.ValueIn).
	// 		WithValue("splitTxOutputAmount", splitTxOut.Value).
	// 		WithValue("inputValueIn", req.input.ValueIn)
	// }

	txFee := splitTxOut.Value - (sess.CommitmentTxOut.Value + sess.ChangeTxOut.Value)
	if txFee < int64(sess.Fee) {
		return ErrFeeTooLow.WithMessagef("Fee too low. Expected=%s but got=%s", sess.Fee, txFee).
			WithValue("expected", sess.Fee).
			WithValue("provided", txFee)
	}
	req.input.ValueIn = wire.NullValueIn

	engine, err := txscript.NewEngine(req.input.SignatureScript, req.splitTx,
		req.splitTxOutputIndex, InputVmValidationFlags, splitTxOut.Version, nil)
	if err != nil {
		return merry.Wrap(err).WithMessage("Error creating engine for split tx validation")
	}
	err = engine.Execute()
	if err != nil {
		return merry.Wrap(err).WithMessage("Error validating participant ticket input against split tx")
	}

	sess.Input = req.input
	sess.SplitTx = req.splitTx
	sess.SplitTxOutputIndex = req.splitTxOutputIndex
	sess.chanPublishTicketResponse = req.resp

	matcher.log.Infof("Setting input for participant %d", req.sessionID)

	if sess.Session.AllInputsFilled() {
		tx, err := sess.Session.CreateTransaction()
		matcher.log.Infof("All inputs received for session")
		for _, p := range sess.Session.Participants {
			p.chanPublishTicketResponse <- publishTicketResponse{
				err: err,
				tx:  tx,
			}
		}

		// FIXME: do publish the transaction on the chain
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
	changeOutput wire.TxOut, voteAddress dcrutil.Address) (*wire.MsgTx, int, error) {

	req := setParticipantOutputsRequest{
		sessionID:        sessionID,
		commitmentOutput: &commitmentOutput,
		changeOutput:     &changeOutput,
		voteAddress:      &voteAddress,
		resp:             make(chan setParticipantOutputsResponse),
	}

	matcher.setParticipantOutputsRequests <- req
	resp := <-req.resp
	return resp.transaction, resp.output_index, resp.err
}

// PublishTransaction validates the signed input provided by one of the
// participants of the given session and publishes the transaction. It blocks
// until all participants have sent their inputs
func (matcher *Matcher) PublishTransaction(sessionID SessionID, splitTx *wire.MsgTx,
	splitTxOutputIndex int, input *wire.TxIn) (*wire.MsgTx, error) {

	req := publishTicketRequest{
		sessionID:          sessionID,
		splitTx:            splitTx,
		input:              input,
		splitTxOutputIndex: splitTxOutputIndex,
		resp:               make(chan publishTicketResponse),
	}

	matcher.publishTicketRequests <- req
	resp := <-req.resp
	return resp.tx, resp.err
}
