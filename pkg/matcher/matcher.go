package matcher

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrwallet/wallet/txrules"
	logging "github.com/op/go-logging"
)

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
	MinAmount                 uint64
	PriceProvider             TicketPriceProvider
	SignPoolSplitOutProvider  SignPoolSplitOutputProvider
	LogLevel                  logging.Level
	LogBackend                logging.LeveledBackend
	ChainParams               *chaincfg.Params
	PoolFee                   float64
	MaxSessionDuration        time.Duration
	StakeDiffChangeStopWindow int32
}

type cancelSessionChanReq struct {
	session *Session
	err     error
}

// Matcher is the main engine for matching operations
type Matcher struct {
	waitingParticipants []*addParticipantRequest
	sessions            map[SessionID]*Session
	participants        map[ParticipantID]*SessionParticipant
	waitingListWatchers map[context.Context]chan []dcrutil.Amount
	cfg                 *Config
	log                 *logging.Logger

	cancelWaitingParticipant      chan addParticipantRequest
	addParticipantRequests        chan addParticipantRequest
	setParticipantOutputsRequests chan setParticipantOutputsRequest
	fundTicketRequests            chan fundTicketRequest
	fundSplitTxRequests           chan fundSplitTxRequest
	cancelSessionChan             chan cancelSessionChanReq
	watchWaitingListRequests      chan watchWaitingListRequest
	cancelWaitingListWatcher      chan context.Context
}

func NewMatcher(cfg *Config) *Matcher {
	m := &Matcher{
		cfg:                 cfg,
		log:                 logging.MustGetLogger("matcher"),
		sessions:            make(map[SessionID]*Session),
		participants:        make(map[ParticipantID]*SessionParticipant),
		waitingListWatchers: make(map[context.Context]chan []dcrutil.Amount),

		addParticipantRequests:        make(chan addParticipantRequest),
		cancelWaitingParticipant:      make(chan addParticipantRequest),
		setParticipantOutputsRequests: make(chan setParticipantOutputsRequest),
		fundTicketRequests:            make(chan fundTicketRequest),
		fundSplitTxRequests:           make(chan fundSplitTxRequest),
		cancelSessionChan:             make(chan cancelSessionChanReq),
		watchWaitingListRequests:      make(chan watchWaitingListRequest),
		cancelWaitingListWatcher:      make(chan context.Context),
	}
	m.log.SetBackend(cfg.LogBackend)

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
			matcher.notifyWaitingListWatchers()
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
		case cancelReq := <-matcher.cancelSessionChan:
			matcher.log.Infof("Cancelling session %s", cancelReq.session.ID)
			cancelReq.session.Canceled = true
			matcher.removeSession(cancelReq.session, cancelReq.err)
		case req := <-matcher.watchWaitingListRequests:
			matcher.log.Debugf("Adding new waiting list watcher")
			matcher.waitingListWatchers[req.ctx] = req.watcher
			go func(c context.Context) {
				<-c.Done()
				matcher.cancelWaitingListWatcher <- c
			}(req.ctx)
		case cancelReq := <-matcher.cancelWaitingListWatcher:
			matcher.log.Debugf("Removing waiting list watcher")
			delete(matcher.waitingListWatchers, cancelReq)
		}
	}
}

func (matcher *Matcher) notifyWaitingListWatchers() {
	amounts := make([]dcrutil.Amount, len(matcher.waitingParticipants))
	for i, p := range matcher.waitingParticipants {
		amounts[i] = dcrutil.Amount(p.maxAmount)
	}

	for ctx, w := range matcher.waitingListWatchers {
		if ctx.Err() == nil {
			select {
			case w <- amounts:
			default:
				matcher.log.Warningf("Possible block when trying to send to waiting list watcher")
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
			<-r.ctx.Done()
			if r.ctx.Err() != nil {
				matcher.cancelWaitingParticipant <- *r
			}
		}(req)
	}

	matcher.notifyWaitingListWatchers()

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
	parts := matcher.waitingParticipants
	matcher.waitingParticipants = nil

	matcher.log.Noticef("Starting new session %s: Ticket Price=%s Fees=%s Participants=%d PoolFee=%s",
		sessID, ticketPrice, ticketTxFee, numParts, dcrutil.Amount(poolFee))

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
		StartTime:      time.Now(),
	}
	matcher.sessions[sessID] = sess

	sort.Sort(addParticipantRequestsByAmount(parts))
	maxAmounts := make([]dcrutil.Amount, len(parts))
	for i, p := range parts {
		maxAmounts[i] = dcrutil.Amount(p.maxAmount)
	}
	commitments, err := SelectContributionAmounts(maxAmounts, ticketPrice, partFee, poolFeePart)
	if err != nil {
		matcher.log.Errorf("Error selecting contribution amounts: %v", err)
		panic(err)
	}

	for i, r := range parts {
		id := matcher.newParticipantID(sessID)
		sessPart := &SessionParticipant{
			CommitAmount: commitments[i],
			PoolFee:      poolFeePart,
			Fee:          partFee,
			Session:      sess,
			Index:        i,
			ID:           id,
		}
		sess.Participants[i] = sessPart
		matcher.participants[id] = sessPart

		r.resp <- addParticipantResponse{
			participant: sessPart,
		}

		matcher.log.Infof("Participant %s contributing with %s", id, commitments[i])
	}

	sess.VoterIndex = ChooseVoter(commitments)
	voter := sess.Participants[sess.VoterIndex]
	matcher.log.Infof("Chosen as voter %s", voter.ID)

	go func(s *Session) {
		sessTimer := time.NewTimer(matcher.cfg.MaxSessionDuration)
		<-sessTimer.C
		if !s.Done && !s.Canceled {
			matcher.log.Warningf("Session %s lasted more than MaxSessionDuration", s.ID)
			matcher.cancelSessionChan <- cancelSessionChanReq{session: s, err: ErrSessionMaxTimeExpired}
		}
	}(sess)
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
			p.sendSetOutputsResponse(setParticipantOutputsResponse{
				ticket:     ticket,
				splitTx:    splitTx,
				revocation: revocation,
				err:        err,
			})
		}

		matcher.log.Infof("Notified participants of created txs for session %s", sess.Session.ID)
	}

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

	if req.ticketInputScriptSig == nil {
		return ErrTicketScriptSigNotProvided
	}

	// TODO: check if revocationScriptSig actually successfully signs the
	// revocation transaction

	// TODO: check if ticketInputScriptSig actually commits to the ticket's outputs
	// and is of the correct sigHashType

	matcher.log.Infof("Participant %s sent ticket input sigs", req.sessionID)

	sess.TicketScriptSig = req.ticketInputScriptSig
	sess.chanFundTicketResponse = req.resp
	sess.RevocationScriptSig = req.revocationScriptSig

	if sess.Session.TicketIsFunded() {
		matcher.log.Infof("All sigscripts for ticket received. Creating funded ticket.")

		ticket, _, revocation, err := sess.Session.CreateTransactions()

		for _, p := range sess.Session.Participants {
			p.sendFundTicketResponse(fundTicketResponse{
				ticket:     ticket,
				revocation: revocation,
				err:        err,
			})
		}

		matcher.log.Infof("Alerted participants of funded ticked on session %s", sess.Session.ID)
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

	matcher.log.Infof("Participant %s sent split tx input sigs", req.sessionID)

	if sess.Session.SplitTxIsFunded() {
		sess.Session.Done = true

		matcher.log.Infof("All inputs for split tx received. Creating split tx.")

		_, splitTx, _, err := sess.Session.CreateTransactions()

		for _, p := range sess.Session.Participants {
			p.sendFundSplitTxResponse(fundSplitTxResponse{
				splitTx: splitTx,
				err:     err,
			})
		}

		matcher.log.Noticef("Session %s successfully finished", sess.Session.ID)
		matcher.removeSession(sess.Session, nil)
	}

	return nil
}

func (matcher *Matcher) cancelSessionOnContextDone(ctx context.Context, sessPart *SessionParticipant) {
	go func() {
		<-ctx.Done()
		sess := sessPart.Session
		if (ctx.Err() != nil) && (!sess.Canceled) && (!sess.Done) {
			matcher.log.Warningf("Cancelling session %s due to context error on participant %s: %v", sess.ID, sessPart.ID, ctx.Err())
			matcher.cancelSessionChan <- cancelSessionChanReq{session: sessPart.Session, err: ErrParticipantDisconnected}
		}
	}()
}

func (matcher *Matcher) removeSession(sess *Session, err error) {
	delete(matcher.sessions, sess.ID)
	for _, p := range sess.Participants {
		if _, has := matcher.participants[p.ID]; has {
			delete(matcher.participants, p.ID)
		}
		if err != nil {
			p.sessionCanceled(err)
		}
	}
}

func (matcher *Matcher) AddParticipant(ctx context.Context, maxAmount uint64) (*SessionParticipant, error) {
	if maxAmount < matcher.cfg.MinAmount {
		return nil, ErrLowAmount
	}

	blockHeight := matcher.cfg.PriceProvider.CurrentBlockHeight()
	stakeDiffChangeDistance := blockHeight % int32(matcher.cfg.ChainParams.WorkDiffWindowSize)
	if (stakeDiffChangeDistance < matcher.cfg.StakeDiffChangeStopWindow) ||
		(stakeDiffChangeDistance > int32(matcher.cfg.ChainParams.WorkDiffWindowSize)-matcher.cfg.StakeDiffChangeStopWindow) {
		return nil, ErrStakeDiffTooCloseToChange
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

func (matcher *Matcher) WatchWaitingList(ctx context.Context, watcher chan []dcrutil.Amount) {
	req := watchWaitingListRequest{
		ctx:     ctx,
		watcher: watcher,
	}

	matcher.watchWaitingListRequests <- req
}

// SetParticipantsOutputs validates and sets the outputs of the given participant
// for the provided outputs, waits for all participants to provide their own
// outputs, then generates the ticket tx and returns the index of the input
// that should receive this participants funds
func (matcher *Matcher) SetParticipantsOutputs(ctx context.Context, sessionID ParticipantID, commitmentOutput,
	changeOutput wire.TxOut, voteAddress dcrutil.Address, splitTxChange wire.TxOut,
	splitTxOutput wire.TxOut, splitTxOutPoints []*wire.OutPoint, poolAddress dcrutil.Address) (*wire.MsgTx, *wire.MsgTx, *wire.MsgTx, error) {

	outpoints := make([]*wire.OutPoint, len(splitTxOutPoints))
	for i, o := range splitTxOutPoints {
		outpoints[i] = o
	}

	req := setParticipantOutputsRequest{
		ctx:              ctx,
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

func (matcher *Matcher) FundTicket(ctx context.Context, sessionID ParticipantID, inputScriptSig []byte, revocationScriptSig []byte) (*wire.MsgTx, *wire.MsgTx, error) {

	req := fundTicketRequest{
		ctx:                  ctx,
		sessionID:            sessionID,
		ticketInputScriptSig: inputScriptSig,
		revocationScriptSig:  revocationScriptSig,
		resp:                 make(chan fundTicketResponse),
	}

	matcher.fundTicketRequests <- req
	resp := <-req.resp
	return resp.ticket, resp.revocation, resp.err
}

func (matcher *Matcher) FundSplit(ctx context.Context, sessionID ParticipantID, inputScriptSigs [][]byte) (*wire.MsgTx, error) {

	req := fundSplitTxRequest{
		ctx:             ctx,
		sessionID:       sessionID,
		inputScriptSigs: inputScriptSigs,
		resp:            make(chan fundSplitTxResponse),
	}
	matcher.fundSplitTxRequests <- req
	resp := <-req.resp
	return resp.splitTx, resp.err

}
