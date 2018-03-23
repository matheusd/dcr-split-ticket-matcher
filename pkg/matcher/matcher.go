package matcher

import (
	"context"
	"fmt"
	"math"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrwallet/wallet/txrules"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/util"
	logging "github.com/op/go-logging"
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

type TicketPriceProvider interface {
	CurrentTicketPrice() uint64
	CurrentBlockHeight() int32
}

type SignPoolSplitOutputProvider interface {
	PoolFeeAddress() dcrutil.Address
	SignPoolSplitOutput(split, ticket *wire.MsgTx) ([]byte, error)
}

type VoteAddressProvider interface {
	VotingAddress() dcrutil.Address
	SignRevocation(ticket, revocation *wire.MsgTx) (*wire.MsgTx, error)
}

// Config stores the parameters for the matcher engine
type Config struct {
	MinAmount                uint64
	MaxOnlineParticipants    int
	PriceProvider            TicketPriceProvider
	VoteAddrProvider         VoteAddressProvider
	SignPoolSplitOutProvider SignPoolSplitOutputProvider
	LogLevel                 logging.Level
	ChainParams              *chaincfg.Params
	PoolFee                  float64
}

// Matcher is the main engine for matching operations
type Matcher struct {
	waitingParticipants []*addParticipantRequest
	sessions            map[SessionID]*Session
	participants        map[ParticipantID]*SessionParticipant
	cfg                 *Config
	log                 *logging.Logger

	cancelWaitingParticipant      chan addParticipantRequest
	addParticipantRequests        chan addParticipantRequest
	setParticipantOutputsRequests chan setParticipantOutputsRequest
	fundTicketRequests            chan fundTicketRequest
	fundSplitTxRequests           chan fundSplitTxRequest
}

func NewMatcher(cfg *Config) *Matcher {
	m := &Matcher{
		cfg:                           cfg,
		log:                           logging.MustGetLogger("matcher"),
		sessions:                      make(map[SessionID]*Session),
		participants:                  make(map[ParticipantID]*SessionParticipant),
		addParticipantRequests:        make(chan addParticipantRequest),
		cancelWaitingParticipant:      make(chan addParticipantRequest),
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
			err := matcher.addParticipant(&req)
			if err != nil {
				req.resp <- addParticipantResponse{
					err: err,
				}
			}
		case cancelReq := <-matcher.cancelWaitingParticipant:
			waiting := matcher.waitingParticipants
			for i, p := range waiting {
				if *p == cancelReq {
					matcher.log.Infof("Dropping waiting participant for %s", dcrutil.Amount(p.maxAmount))
					matcher.waitingParticipants = append(waiting[:i], waiting[i+1:]...)
				}
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

func (matcher *Matcher) addParticipant(req *addParticipantRequest) error {
	matcher.log.Infof("Adding participant for amount %s", dcrutil.Amount(req.maxAmount))
	matcher.waitingParticipants = append(matcher.waitingParticipants, req)
	if matcher.enoughForNewSession() {
		matcher.startNewSession()
	} else {
		go func(r *addParticipantRequest) {
			<-req.ctx.Done()
			if r.ctx.Err() != nil {
				matcher.cancelWaitingParticipant <- *r
			}
		}(req)
	}

	return nil
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
	partFee := SessionParticipantFee(numParts)
	ticketTxFee := partFee * dcrutil.Amount(numParts)
	ticketPrice := dcrutil.Amount(matcher.cfg.PriceProvider.CurrentTicketPrice())
	blockHeight := matcher.cfg.PriceProvider.CurrentBlockHeight()
	poolFeePerc := matcher.cfg.PoolFee
	poolFee := txrules.StakePoolTicketFee(ticketPrice, ticketTxFee, blockHeight,
		poolFeePerc, matcher.cfg.ChainParams)
	poolFeePart := dcrutil.Amount(math.Ceil(float64(poolFee) / float64(numParts)))
	sessID := matcher.newSessionID()

	matcher.log.Infof("Starting new session %s: Ticket Price=%s Fees=%s Participants=%d PoolFee=%d",
		sessID, ticketPrice, ticketTxFee, numParts, poolFee)

	splitPoolOutAddr := matcher.cfg.SignPoolSplitOutProvider.PoolFeeAddress()
	splitPoolOutScript, err := txscript.PayToAddrScript(splitPoolOutAddr)
	if err != nil {
		matcher.log.Errorf("Error generating splitPoolOutScript: %v", err)
		panic(err)
	}

	sess := &Session{
		Participants:   make([]*SessionParticipant, numParts),
		TicketPrice:    dcrutil.Amount(matcher.cfg.PriceProvider.CurrentTicketPrice()),
		PoolFee:        poolFee,
		ChainParams:    matcher.cfg.ChainParams,
		TicketPoolIn:   wire.NewTxIn(&wire.OutPoint{Index: 0}, nil),
		SplitTxPoolOut: wire.NewTxOut(int64(poolFee), splitPoolOutScript),
		ID:             sessID,
	}
	matcher.sessions[sessID] = sess

	contributions := make([]dcrutil.Amount, numParts)
	commitLeft := ticketPrice - poolFee
	for i, r := range matcher.waitingParticipants {
		commitAmount := dcrutil.Amount(r.maxAmount) - partFee - poolFeePart
		if commitAmount > commitLeft {
			commitAmount = commitLeft
		}

		id := matcher.newParticipantID(sessID)
		sessPart := &SessionParticipant{
			CommitAmount: commitAmount,
			PoolFee:      poolFeePart,
			Fee:          partFee,
			Session:      sess,
			Index:        i,
			ID:           id,
		}
		sess.Participants[i] = sessPart
		contributions[i] = commitAmount

		matcher.participants[id] = sessPart

		r.resp <- addParticipantResponse{
			participant: sessPart,
		}

		commitLeft -= commitAmount

		matcher.log.Infof("Participant %s contributing with %s", id, commitAmount)
	}

	sess.VoterIndex = ChooseVoter(contributions)
	voter := sess.Participants[sess.VoterIndex]
	matcher.log.Infof("Chosen as voter %s", voter.ID)

	matcher.waitingParticipants = nil
}

func (matcher *Matcher) newSessionID() SessionID {
	// TODO: rw lock matcher.sessions here
	id := SessionID(MustRandUInt16())
	for _, has := matcher.sessions[id]; has; {
		id = SessionID(MustRandUInt16())
	}
	return id
}

func (matcher *Matcher) newParticipantID(sessionID SessionID) ParticipantID {
	// TODO rw lock matcher.participants
	upper := ParticipantID(uint32(sessionID) << 16)
	id := upper | ParticipantID(MustRandUInt16())
	for _, has := matcher.participants[id]; has; {
		id = upper | ParticipantID(MustRandUInt16())
	}
	return id
}

func (matcher *Matcher) setParticipantsOutputs(req *setParticipantOutputsRequest) error {

	if _, has := matcher.participants[req.sessionID]; !has {
		return ErrSessionNotFound.WithMessagef("Session with ID %s not found", req.sessionID)
	}

	if req.commitmentOutput == nil {
		return ErrNilCommitmentOutput
	}

	if req.changeOutput == nil {
		return ErrNilChangeOutput
	}

	sess := matcher.participants[req.sessionID]
	// TODO: decode and check commitment amount
	// if req.commitmentOutput.Value != int64(sess.Amount) {
	// 	return ErrCommitmentValueDifferent.
	// 		WithMessagef("Commitment amount %s different then expected %s", dcrutil.Amount(req.commitmentOutput.Value), sess.Amount).
	// 		WithValue("expectedAmount", sess.Amount).
	// 		WithValue("providedAmount", req.commitmentOutput.Value)
	// }
	if req.splitTxOutput.Value != int64(sess.CommitAmount+sess.Fee) {
		return ErrSplitValueInputValueMismatch.
			WithMessagef("Split txOut.value (%s) !== session.amount (%s) + session.fee(%s)", dcrutil.Amount(req.splitTxOutput.Value), sess.CommitAmount, sess.Fee)
	}

	if len(req.splitTxOutPoints) == 0 {
		return ErrNoSplitTxInputOutPoints
	}

	if req.voteAddress == nil {
		return ErrVoteAddressNotSpecified
	}

	if req.poolAddress == nil {
		return ErrNoPoolAddress
	}

	// TODO: get the utxos from network and validate whether the sum(utxos) == splitTxOut + splitTxChange + SplitTxFee

	matcher.log.Infof("Participant %s set output commitment %s", req.sessionID, sess.CommitAmount)

	sess.CommitmentTxOut = req.commitmentOutput
	sess.ChangeTxOut = req.changeOutput
	sess.chanSetOutputsResponse = req.resp
	sess.SplitTxOut = req.splitTxOutput
	sess.SplitTxChange = req.splitTxChange
	sess.SplitTxInputs = make([]*wire.TxIn, len(req.splitTxOutPoints))
	sess.VoteAddress = req.voteAddress
	sess.PoolAddress = req.poolAddress
	for i, outp := range req.splitTxOutPoints {
		sess.SplitTxInputs[i] = wire.NewTxIn(outp, nil)
	}

	if sess.Session.AllOutputsFilled() {
		matcher.log.Infof("All outputs for session received. Creating txs.")

		var ticket, splitTx, revocation *wire.MsgTx
		var poolTicketInSig []byte
		var err error

		ticket, splitTx, revocation, err = sess.Session.CreateTransactions()
		if err == nil {
			poolTicketInSig, err = matcher.cfg.SignPoolSplitOutProvider.SignPoolSplitOutput(splitTx, ticket)
			if err == nil {
				sess.Session.TicketPoolIn.SignatureScript = poolTicketInSig
			} else {
				matcher.log.Errorf("Error signing pool split input: %v", err)
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

	matcher.log.Infof("Notified participants of created txs")

	return nil
}

func (matcher *Matcher) fundTicket(req *fundTicketRequest) error {
	if _, has := matcher.participants[req.sessionID]; !has {
		return ErrSessionNotFound
	}

	sess := matcher.participants[req.sessionID]
	if sess.ChangeTxOut == nil {
		return ErrNilChangeOutput
	}

	if sess.CommitmentTxOut == nil {
		return ErrNilCommitmentOutput
	}

	if (sess.Index == sess.Session.VoterIndex) && (req.revocationScriptSig == nil) {
		return ErrNoRevocationScriptSig
	}

	// TODO: check if revocationScriptSig actually successfully signs the
	// revocation transaction

	// TODO: check if ticketInputScriptSig actually commits to the ticket's outputs
	// and is of the correct sigHashType

	sess.TicketScriptSig = req.ticketInputScriptSig
	sess.chanFundTicketResponse = req.resp
	sess.RevocationScriptSig = req.revocationScriptSig

	if sess.Session.TicketIsFunded() {
		matcher.log.Infof("All sigscripts for ticket received. Creating funded ticket.")

		ticket, _, revocation, err := sess.Session.CreateTransactions()

		for _, p := range sess.Session.Participants {
			p.chanFundTicketResponse <- fundTicketResponse{
				ticket:     ticket,
				revocation: revocation,
				err:        err,
			}
			matcher.log.Infof("Alerted participant %s of funded ticked", p.ID)
		}
	}

	return nil
}

func (matcher *Matcher) fundSplitTx(req *fundSplitTxRequest) error {
	if _, has := matcher.participants[req.sessionID]; !has {
		return ErrSessionNotFound
	}
	sess := matcher.participants[req.sessionID]

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

		_, splitTx, _, err := sess.Session.CreateTransactions()

		for _, p := range sess.Session.Participants {
			p.chanFundSplitTxResponse <- fundSplitTxResponse{
				splitTx: splitTx,
				err:     err,
			}
			matcher.log.Infof("Alerted participant %s of funded split tx", p.ID)
		}
	}

	matcher.log.Noticef("Session %s successfully finished", sess.Session.ID)

	return nil
}

func (matcher *Matcher) AddParticipant(ctx context.Context, maxAmount uint64) (*SessionParticipant, error) {
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
		ctx:       ctx,
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
func (matcher *Matcher) SetParticipantsOutputs(sessionID ParticipantID, commitmentOutput,
	changeOutput wire.TxOut, voteAddress dcrutil.Address, splitTxChange wire.TxOut,
	splitTxOutput wire.TxOut, splitTxOutPoints []*wire.OutPoint, poolAddress dcrutil.Address) (*wire.MsgTx, *wire.MsgTx, *wire.MsgTx, error) {

	outpoints := make([]*wire.OutPoint, len(splitTxOutPoints))
	for i, o := range splitTxOutPoints {
		outpoints[i] = o
	}

	req := setParticipantOutputsRequest{
		sessionID:        sessionID,
		commitmentOutput: &commitmentOutput,
		changeOutput:     &changeOutput,
		voteAddress:      &voteAddress,
		poolAddress:      &poolAddress,
		splitTxChange:    &splitTxChange,
		splitTxOutput:    &splitTxOutput,
		splitTxOutPoints: outpoints,
		resp:             make(chan setParticipantOutputsResponse),
	}

	matcher.setParticipantOutputsRequests <- req
	resp := <-req.resp
	return resp.ticket, resp.splitTx, resp.revocation, resp.err
}

func (matcher *Matcher) FundTicket(sessionID ParticipantID, inputScriptSig []byte, revocationScriptSig []byte) (*wire.MsgTx, *wire.MsgTx, error) {

	req := fundTicketRequest{
		sessionID:            sessionID,
		ticketInputScriptSig: inputScriptSig,
		revocationScriptSig:  revocationScriptSig,
		resp:                 make(chan fundTicketResponse),
	}

	matcher.fundTicketRequests <- req
	resp := <-req.resp
	return resp.ticket, resp.revocation, resp.err
}

func (matcher *Matcher) FundSplit(sessionID ParticipantID, inputScriptSigs [][]byte) (*wire.MsgTx, error) {

	req := fundSplitTxRequest{
		sessionID:       sessionID,
		inputScriptSigs: inputScriptSigs,
		resp:            make(chan fundSplitTxResponse),
	}
	matcher.fundSplitTxRequests <- req
	resp := <-req.resp
	return resp.splitTx, resp.err

}
