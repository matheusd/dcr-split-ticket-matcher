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

// Participant is someone that can participate on a ticket split
type Participant struct {
	MaxAmount dcrutil.Amount
}

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
type SessionID uint64

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
			tx.AddTxOut(p.ChangeTxOut)
		}

		if p.ChangeTxOut != nil {
			tx.AddTxOut(p.ChangeTxOut)
		}
	}

	return tx, nil
}

type TicketPriceProvider interface {
	CurrentTicketPrice() dcrutil.Amount
}

// Config stores the parameters for the matcher engine
type Config struct {
	MinAmount             dcrutil.Amount
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
	publishSessionRequests        chan publishSessionRequest
}

func RunMatcher(cfg *Config) error {
	m := &Matcher{
		cfg: cfg,
		log: logging.MustGetLogger("matcher"),
	}
	util.SetLoggerBackend(true, "", "", cfg.LogLevel, m.log)

	m.addParticipantRequests = make(chan addParticipantRequest, cfg.MaxOnlineParticipants)

	return m.run()
}

func (matcher *Matcher) run() error {
	for {
		select {
		case req := <-matcher.addParticipantRequests:
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
	return nil
}

func (matcher *Matcher) enoughForNewSession() bool {

	ticketPrice := matcher.cfg.PriceProvider.CurrentTicketPrice()
	availableSum := dcrutil.Amount(0)

	for _, r := range matcher.waitingParticipants {
		availableSum += r.participant.MaxAmount
	}

	ticketFee := SessionFeeEstimate(len(matcher.waitingParticipants))
	neededAmount := ticketPrice + ticketFee
	return availableSum > neededAmount
}

func (matcher *Matcher) startNewSession() {
	numParts := len(matcher.waitingParticipants)
	ticketFee := SessionFeeEstimate(numParts)
	partFee := dcrutil.Amount(math.Ceil(float64(ticketFee) / float64(numParts)))

	sess := &Session{
		Participants: make([]*SessionParticipant, numParts),
		TicketPrice:  matcher.cfg.PriceProvider.CurrentTicketPrice(),
		VoterIndex:   0, // FIXME: select voter index at random
	}

	amountLeft := matcher.cfg.PriceProvider.CurrentTicketPrice()
	for i, r := range matcher.waitingParticipants {
		amount := r.participant.MaxAmount - partFee
		if amount > amountLeft {
			amount = amountLeft
		}

		sessPart := &SessionParticipant{
			Amount:  amount,
			Fee:     partFee,
			Session: sess,
			Index:   i,
		}
		sess.Participants = append(sess.Participants, sessPart)

		id := matcher.newSessionID()
		matcher.sessions[id] = sessPart
		sessPart.ID = id

		r.resp <- addParticipantResponse{
			participant: sessPart,
		}
	}

	matcher.waitingParticipants = nil
}

func (matcher *Matcher) newSessionID() SessionID {
	// TODO: rw lock matcher.sessions here
	id := MustRandUint64()
	for _, has := matcher.sessions[SessionID(id)]; has; {
		id = MustRandUint64()
	}
	return SessionID(id)
}

func (matcher *Matcher) setParticipantsOutputs(req *setParticipantOutputsRequest) error {
	if _, has := matcher.sessions[req.sessionID]; !has {
		return ErrSessionNotFound
	}

	if req.commitmentOutput == nil {
		return ErrNilCommitmentOutput
	}

	if req.changeOutput == nil {
		return ErrNilChangeOutput
	}

	sess := matcher.sessions[req.sessionID]
	if req.commitmentOutput.Value != int64(sess.Amount) {
		return ErrCommitmentValueDifferent.
			WithValue("expectedAmount", sess.Amount).
			WithValue("providedAmount", req.commitmentOutput.Value)
	}

	sess.CommitmentTxOut = req.commitmentOutput
	sess.ChangeTxOut = req.changeOutput
	sess.chanSetOutputsResponse = req.resp

	if sess.Session.AllOutputsFilled() {
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

	txFee := req.input.ValueIn - (sess.CommitmentTxOut.Value + sess.ChangeTxOut.Value)
	if txFee < int64(sess.Fee) {
		return ErrFeeTooLow.
			WithValue("expected", sess.Fee).
			WithValue("provided", txFee)
	}

	if req.splitTxOutputIndex >= len(req.splitTx.TxOut) {
		return ErrIndexNotFound.WithValue("index", req.splitTxOutputIndex)
	}

	splitTxOut := req.splitTx.TxOut[req.splitTxOutputIndex]
	if splitTxOut.Value != req.input.ValueIn {
		return ErrSplitValueInputValueMismatch.
			WithValue("splitTxOutputAmount", splitTxOut.Value).
			WithValue("inputValueIn", req.input.ValueIn)
	}

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

	if sess.Session.AllInputsFilled() {
		tx, err := sess.Session.CreateTransaction()
		for _, p := range sess.Session.Participants {
			p.chanPublishTicketResponse <- publishTicketResponse{
				err: err,
				tx:  tx,
			}
		}

		matcher.publishSessionRequests <- publishSessionRequest{
			session: sess.Session,
		}
	}

	return nil
}

func (matcher *Matcher) AddParticipant(p *Participant) (*SessionParticipant, error) {
	if p.MaxAmount < matcher.cfg.MinAmount {
		return nil, ErrLowAmount
	}

	// TODO: not really great. Should be automatically calc'd by the ticket
	// fee + minimum amount for each participant (which is determined by
	// ticket fee + max amount of commitments in an SSTX)
	if len(matcher.waitingParticipants) >= matcher.cfg.MaxOnlineParticipants {
		return nil, ErrTooManyParticipants
	}

	req := addParticipantRequest{
		participant: p,
		resp:        make(chan addParticipantResponse),
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
