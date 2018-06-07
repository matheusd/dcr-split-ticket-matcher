package matcher

import (
	"context"
	"encoding/hex"
	"math"
	"sort"
	"time"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrwallet/wallet/txrules"
	logging "github.com/op/go-logging"
	"github.com/pkg/errors"
)

// NetworkProvider is the interface for operations that the matcher service
// needs from the decred network. These operations might be provided by a
// connection to a full node or to an external api (such as dcrdata).
type NetworkProvider interface {
	CurrentTicketPrice() uint64
	CurrentBlockHeight() uint32
	CurrentBlockHash() chainhash.Hash
	ConnectedToDecredNetwork() bool
	PublishTransactions([]*wire.MsgTx) error
	GetUtxos(outpoints []*wire.OutPoint) (splitticket.UtxoMap, error)
}

// SignPoolSplitOutputProvider is the interface for the poerations the matcher
// needs for generating and signing the pool fee address input of tickets.
type SignPoolSplitOutputProvider interface {
	PoolFeeAddress() dcrutil.Address
	SignPoolSplitOutput(split, ticket *wire.MsgTx) ([]byte, error)
}

// VoteAddressValidationProvider is the interface for operations the matcher needs to
// validate if a given vote address is valid. Implementations
// should return nil if the address is valid or an error otherwise.
type VoteAddressValidationProvider interface {
	ValidateVoteAddress(voteAddr dcrutil.Address) error
}

// PoolAddressValidationProvider is the interface for operations the matcher
// needs to validate if a given pool fee/pool subsidy address is valid.
// Implementation should return nil if the address is valid or an error
// otherwise.
type PoolAddressValidationProvider interface {
	ValidatePoolSubsidyAddress(poolAddr dcrutil.Address) error
}

// Config stores the parameters for the matcher engine
type Config struct {
	MinAmount                 uint64
	NetworkProvider           NetworkProvider
	SignPoolSplitOutProvider  SignPoolSplitOutputProvider
	VoteAddrValidator         VoteAddressValidationProvider
	PoolAddrValidator         PoolAddressValidationProvider
	LogLevel                  logging.Level
	LogBackend                logging.LeveledBackend
	ChainParams               *chaincfg.Params
	PoolFee                   float64
	MaxSessionDuration        time.Duration
	StakeDiffChangeStopWindow int32
	PublishTransactions       bool
	SessionDataDir            string
}

type cancelSessionChanReq struct {
	session *Session
	err     error
}

type ParticipantTicketOutput struct {
	SecretHash   *splitticket.SecretNumberHash
	VotePkScript []byte
	PoolPkScript []byte
	Amount       dcrutil.Amount
}

type WaitingQueue struct {
	Name    string
	Amounts []dcrutil.Amount
}

// Matcher is the main engine for matching operations
type Matcher struct {
	queues              map[string]*splitTicketQueue
	sessions            map[SessionID]*Session
	participants        map[ParticipantID]*SessionParticipant
	waitingListWatchers map[context.Context]chan []WaitingQueue
	cfg                 *Config
	log                 *logging.Logger

	cancelWaitingParticipant      chan *addParticipantRequest
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
		queues:              make(map[string]*splitTicketQueue),
		log:                 logging.MustGetLogger("matcher"),
		sessions:            make(map[SessionID]*Session),
		participants:        make(map[ParticipantID]*SessionParticipant),
		waitingListWatchers: make(map[context.Context]chan []WaitingQueue),

		addParticipantRequests:        make(chan addParticipantRequest),
		cancelWaitingParticipant:      make(chan *addParticipantRequest),
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
			q, has := matcher.queues[cancelReq.sessionName]
			if has {
				matcher.log.Infof("Dropping waiting participant of queue '%s' for %s",
					cancelReq.sessionName, dcrutil.Amount(cancelReq.maxAmount))
				q.removeWaitingParticipant(cancelReq)
				if q.empty() {
					delete(matcher.queues, cancelReq.sessionName)
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
	queues := make([]WaitingQueue, len(matcher.queues))
	i := 0
	for name, q := range matcher.queues {
		queues[i] = WaitingQueue{
			Name:    name,
			Amounts: q.waitingAmounts(),
		}
		i++
	}

	for ctx, w := range matcher.waitingListWatchers {
		if ctx.Err() == nil {
			select {
			case w <- queues:
			default:
				matcher.log.Warningf("Possible block when trying to send to waiting list watcher")
			}
		}
	}
}

func (matcher *Matcher) addParticipant(req *addParticipantRequest) error {
	matcher.log.Infof("Adding participant for amount %s on queue %s",
		dcrutil.Amount(req.maxAmount), req.sessionName)
	q, has := matcher.queues[req.sessionName]
	if !has {
		q = newSplitTicketQueue(matcher.cfg.NetworkProvider)
		matcher.queues[req.sessionName] = q
	}

	q.addWaitingParticipant(req)

	if q.enoughForNewSession() {
		delete(matcher.queues, req.sessionName)
		matcher.startNewSession(q)
	} else {
		go func(r *addParticipantRequest) {
			<-r.ctx.Done()
			if r.ctx.Err() != nil {
				matcher.cancelWaitingParticipant <- r
			}
		}(req)
	}

	matcher.notifyWaitingListWatchers()

	return nil
}

func (matcher *Matcher) startNewSession(q *splitTicketQueue) {
	numParts := len(q.waitingParticipants)
	partFee := SessionParticipantFee(numParts)
	ticketTxFee := partFee * dcrutil.Amount(numParts)
	ticketPrice := dcrutil.Amount(matcher.cfg.NetworkProvider.CurrentTicketPrice())
	blockHeight := matcher.cfg.NetworkProvider.CurrentBlockHeight()
	poolFeePerc := matcher.cfg.PoolFee
	minPoolFee := txrules.StakePoolTicketFee(ticketPrice, ticketTxFee, int32(blockHeight),
		poolFeePerc, matcher.cfg.ChainParams)
	poolFeePart := dcrutil.Amount(math.Ceil(float64(minPoolFee) / float64(numParts)))
	poolFee := poolFeePart * dcrutil.Amount(numParts) // ensure every participant pays the same amount
	sessID := matcher.newSessionID()
	parts := q.waitingParticipants
	curHeight := matcher.cfg.NetworkProvider.CurrentBlockHeight()
	expiry := TargetTicketExpirationBlock(curHeight, MaximumExpiry,
		matcher.cfg.ChainParams)
	q.waitingParticipants = nil

	matcher.log.Noticef("Starting new session %s: Ticket Price=%s Fees=%s Participants=%d PoolFee=%s",
		sessID, ticketPrice, ticketTxFee, numParts, dcrutil.Amount(poolFee))

	splitPoolOutAddr := matcher.cfg.SignPoolSplitOutProvider.PoolFeeAddress()
	splitPoolOutScript, err := txscript.PayToAddrScript(splitPoolOutAddr)
	if err != nil {
		matcher.log.Errorf("Error generating splitPoolOutScript: %v", err)
		panic(err)
	}

	sess := &Session{
		Participants:    make([]*SessionParticipant, numParts),
		TicketPrice:     dcrutil.Amount(matcher.cfg.NetworkProvider.CurrentTicketPrice()),
		MainchainHash:   matcher.cfg.NetworkProvider.CurrentBlockHash(),
		MainchainHeight: curHeight,
		PoolFee:         poolFee,
		TicketFee:       ticketTxFee,
		ChainParams:     matcher.cfg.ChainParams,
		TicketPoolIn:    wire.NewTxIn(&wire.OutPoint{Index: 1}, nil),       // FIXME: this should probably be removed from here and moved into the session
		SplitTxPoolOut:  wire.NewTxOut(int64(poolFee), splitPoolOutScript), // ditto above
		ID:              sessID,
		StartTime:       time.Now(),
		TicketExpiry:    expiry,
		VoterIndex:      -1, // voter not decided yet
	}
	matcher.sessions[sessID] = sess

	sort.Sort(addParticipantRequestsByAmount(parts))
	maxAmounts := make([]dcrutil.Amount, len(parts))
	for i, p := range parts {
		maxAmounts[i] = dcrutil.Amount(p.maxAmount)
	}
	commitments, err := splitticket.SelectContributionAmounts(maxAmounts, ticketPrice, partFee, poolFeePart)
	if err != nil {
		matcher.log.Errorf("Error selecting contribution amounts: %v", err)
		panic(err)
	}

	// TODO: shuffle the participants after creating the sess.Participants array
	// but before sending the reply.

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

	go func(s *Session) {
		sessTimer := time.NewTimer(matcher.cfg.MaxSessionDuration)
		<-sessTimer.C
		if !s.Done && !s.Canceled {
			matcher.log.Warningf("Session %s lasted more than MaxSessionDuration", s.ID)
			matcher.cancelSessionChan <- cancelSessionChanReq{session: s,
				err: errors.Errorf("session expired")}
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

	var err error

	if _, has := matcher.participants[req.sessionID]; !has {
		return errors.Errorf("session %s not found", req.sessionID.String())
	}
	part := matcher.participants[req.sessionID]

	utxos, err := matcher.cfg.NetworkProvider.GetUtxos(req.splitTxOutPoints)
	if err != nil {
		return errors.Wrapf(err, "error obtaining utxos")
	}

	var changeAmount dcrutil.Amount
	if req.splitTxChange != nil {
		changeAmount = dcrutil.Amount(req.splitTxChange.Value)
	}

	var inputAmount dcrutil.Amount
	for _, utxo := range utxos {
		inputAmount += utxo.Value
	}

	// FIXME: take into account estimate for split tx fees

	// this checks whether the **total** input amount is consistent
	expectedInputAmount := part.CommitAmount + part.PoolFee + part.Fee + changeAmount
	if inputAmount < expectedInputAmount {
		return errors.Errorf("total input amount (%s) less than the expected "+
			"(%s)", inputAmount, expectedInputAmount)
	}

	// this checks whether the **participation** input amount (whatever is input
	// and not sent to change) is consistent
	expectedInputAmount = part.CommitAmount + part.PoolFee + part.Fee
	if inputAmount-changeAmount < expectedInputAmount {
		return errors.Errorf("participation input amount (%s) less than the "+
			"expected (%s)", (inputAmount - changeAmount).String(),
			expectedInputAmount.String())
	}

	err = matcher.cfg.VoteAddrValidator.ValidateVoteAddress(req.voteAddress)
	if err != nil {
		matcher.log.Errorf("Participant %s sent invalid vote address: %s",
			part.ID.String(), err)
		return errors.Wrapf(err, "invalid vote address")
	}

	err = matcher.cfg.PoolAddrValidator.ValidatePoolSubsidyAddress(req.poolAddress)
	if err != nil {
		matcher.log.Errorf("Participant %s sent invalid pool address: %s",
			part.ID.String(), err)
		return errors.Wrapf(err, "invalid pool address")
	}

	part.VoteAddress = req.voteAddress
	part.PoolAddress = req.poolAddress
	part.CommitmentAddress = req.commitAddress
	part.SplitTxAddress = req.splitTxAddress
	part.SecretHash = req.secretHash
	part.splitTxChange = req.splitTxChange
	part.splitTxInputs = make([]*wire.TxIn, len(req.splitTxOutPoints))
	part.splitTxUtxos = utxos
	for i, outp := range req.splitTxOutPoints {
		part.splitTxInputs[i] = wire.NewTxIn(outp, nil)
	}
	part.chanSetOutputsResponse = req.resp

	if err = part.createIOs(); err != nil {
		return errors.Wrap(err, "error creating IOs for participant")
	}

	matcher.log.Infof("Participant %s set output commitment %s", req.sessionID,
		part.CommitAmount)

	sess := part.Session

	if sess.AllOutputsFilled() {
		matcher.log.Infof("All outputs for session received. Creating txs.")

		var ticket, splitTx *wire.MsgTx
		var poolTicketInSig []byte
		var splitUtxos splitticket.UtxoMap
		var err error

		ticket, splitTx, err = sess.CreateTransactions()
		if err == nil {
			splitUtxos, err = sess.SplitUtxoMap()
		}
		if err == nil {
			err = splitticket.CheckSplit(splitTx, splitUtxos, sess.SecretNumberHashes(),
				&sess.MainchainHash, sess.MainchainHeight, matcher.cfg.ChainParams)
			if err == nil {
				err = splitticket.CheckTicket(splitTx, ticket, sess.TicketPrice,
					part.PoolFee, part.Fee, sess.ParticipantAmounts(),
					sess.MainchainHeight, matcher.cfg.ChainParams)
				if err != nil {
					matcher.log.Errorf("error checking ticket: %v", err)
				}
			} else {
				matcher.log.Errorf("error checking split tx: %v", err)
			}
		} else {
			matcher.log.Errorf("error obtaining utxo map for session: %v", err)
		}

		if err == nil {
			toSign := ticket.Copy()
			for _, p := range sess.Participants {
				p.replaceTicketIOs(toSign)
				poolTicketInSig, err = matcher.cfg.SignPoolSplitOutProvider.SignPoolSplitOutput(splitTx, toSign)
				if err != nil {
					matcher.log.Errorf("Error signing pool fee output: %v", err)
					return err
				}
				p.poolFeeInputScriptSig = poolTicketInSig
			}

		} else {
			matcher.log.Errorf("Error generating session transactions: %v", err)
		}

		parts := sess.ParticipantTicketOutputs()

		for _, p := range sess.Participants {
			p.sendSetOutputsResponse(setParticipantOutputsResponse{
				ticket:       ticket,
				splitTx:      splitTx,
				participants: parts,
				index:        uint32(p.Index),
				err:          err,
			})
		}

		matcher.log.Infof("Notified participants of created txs for session %s",
			sess.ID)
	}

	return nil
}

func (matcher *Matcher) fundTicket(req *fundTicketRequest) error {
	if _, has := matcher.participants[req.sessionID]; !has {
		return errors.Errorf("session %s not found", req.sessionID.String())
	}

	part := matcher.participants[req.sessionID]
	sess := part.Session

	if len(req.ticketsInputScriptSig) != len(sess.Participants) {
		return errors.Errorf("size of ticketInputScriptSig (%d) different "+
			"than the number of participants (%d)",
			len(req.ticketsInputScriptSig), len(sess.Participants))
	}

	// TODO: check if revocationScriptSig actually successfully signs the
	// revocation transaction

	// TODO: check if ticketInputScriptSig actually commits to the ticket's outputs

	matcher.log.Infof("Participant %s sent ticket input sigs", req.sessionID)

	part.ticketsScriptSig = req.ticketsInputScriptSig
	part.revocationScriptSig = req.revocationScriptSig
	part.chanFundTicketResponse = req.resp

	if sess.TicketIsFunded() {
		matcher.log.Infof("All sigscripts for ticket received. Creating  " +
			"funded ticket.")

		var ticketHash chainhash.Hash

		// create one ticket for each participant
		tickets := make([][]byte, len(sess.Participants))
		revocations := make([][]byte, len(tickets))

		ticket, _, err := sess.CreateTransactions()

		for i, p := range sess.Participants {
			p.replaceTicketIOs(ticket)
			ticketHash = ticket.TxHash()
			revocation, err := CreateUnsignedRevocation(&ticketHash, ticket,
				dcrutil.Amount(RevocationFeeRate))
			if err != nil {
				matcher.log.Errorf("Error creating participant's revocation: %v", err)
				return err
			}

			p.replaceRevocationInput(ticket, revocation)

			buff, err := ticket.Bytes()
			if err != nil {
				matcher.log.Errorf("Error serializing ticket with changed IOs: %v", err)
				return err
			}

			buffRevocation, err := revocation.Bytes()
			if err != nil {
				matcher.log.Errorf("Error serializing revocation with changed input: %v", err)
				return err
			}

			tickets[i] = buff
			revocations[i] = buffRevocation
		}

		for _, p := range sess.Participants {
			p.sendFundTicketResponse(fundTicketResponse{
				tickets:     tickets,
				revocations: revocations,
				err:         err,
			})
		}

		matcher.log.Infof("Alerted participants of funded ticked on session %s",
			sess.ID)
	}

	return nil
}

func (matcher *Matcher) fundSplitTx(req *fundSplitTxRequest) error {
	if _, has := matcher.participants[req.sessionID]; !has {
		return errors.Errorf("session %s not found", req.sessionID.String())
	}
	part := matcher.participants[req.sessionID]
	sess := part.Session

	if len(part.splitTxInputs) != len(req.inputScriptSigs) {
		return errors.Errorf("len(splitTxInputs %d) != len(inputScriptSigs %d)",
			len(part.splitTxInputs), len(req.inputScriptSigs))
	}

	sentNbHash := req.secretNb.Hash(&sess.MainchainHash)
	if !sentNbHash.Equals(part.SecretHash) {
		return errors.Errorf("disclosed secret number (%d) does not hash "+
			"(%s) to previously sent hash (%s)", req.secretNb,
			hex.EncodeToString(sentNbHash[:]),
			hex.EncodeToString(part.SecretHash[:]))
	}

	// TODO: make sure the script sigs actually sign the split tx

	part.SecretNb = req.secretNb
	for i, script := range req.inputScriptSigs {
		part.splitTxInputs[i].SignatureScript = script
	}
	part.chanFundSplitTxResponse = req.resp

	matcher.log.Infof("Participant %s sent split tx input sigs", req.sessionID)

	if sess.SplitTxIsFunded() {
		var splitBytes []byte
		var err error

		selCoin, selIndex := sess.FindVoterCoinIndex()
		sess.Done = true
		sess.VoterIndex = selIndex
		sess.SelectedCoin = selCoin
		voter := sess.Participants[sess.VoterIndex]

		matcher.log.Infof("All inputs for split tx received. Creating split tx.")
		matcher.log.Infof("Voter index selected: %d (%s)", sess.VoterIndex,
			voter.ID)

		ticket, splitTx, revocation, err := sess.CreateVoterTransactions()
		if err != nil {
			matcher.log.Errorf("error generating voter txs: %v", err)
			return err
		}
		secrets := sess.SecretNumbers()

		// we ignore the error here because this doesn't change from setParticipantOutputs()
		splitUtxoMap, _ := sess.SplitUtxoMap()
		err = splitticket.CheckSplit(splitTx, splitUtxoMap,
			sess.SecretNumberHashes(), &sess.MainchainHash, sess.MainchainHeight,
			matcher.cfg.ChainParams)
		if err != nil {
			matcher.log.Errorf("error on final checkSplit: %v", err)
		}

		err = splitticket.CheckSignedSplit(splitTx, splitUtxoMap,
			matcher.cfg.ChainParams)
		if err != nil {
			matcher.log.Errorf("error on final checkSignedSplit: %v", err)
		}

		err = splitticket.CheckTicket(splitTx, ticket, sess.TicketPrice,
			part.PoolFee, part.Fee, sess.ParticipantAmounts(),
			sess.MainchainHeight, matcher.cfg.ChainParams)
		if err != nil {
			matcher.log.Errorf("error on final checkTicket: %v", err)
		}

		err = splitticket.CheckSignedTicket(splitTx, ticket,
			matcher.cfg.ChainParams)
		if err != nil {
			matcher.log.Errorf("error on final checkSignedTicket: %v", err)
		}

		err = splitticket.CheckRevocation(ticket, revocation,
			matcher.cfg.ChainParams)
		if err != nil {
			matcher.log.Errorf("error on final checkRevocation: %v", err)
		}

		if matcher.cfg.PublishTransactions {
			txs := []*wire.MsgTx{splitTx, ticket}
			matcher.log.Infof("Publishing transactions of session %s", sess.ID)
			err = matcher.cfg.NetworkProvider.PublishTransactions(txs)
			if err != nil {
				matcher.log.Errorf("Error publishing transactions: %s", err)
			}
		} else {
			matcher.log.Infof("Skipping publishing transactions")
		}

		if err == nil {
			splitBytes, err = splitTx.Bytes()
		}

		for _, p := range sess.Participants {
			p.sendFundSplitTxResponse(fundSplitTxResponse{
				splitTx: splitBytes,
				secrets: secrets,
				err:     err,
			})
		}

		if matcher.cfg.SessionDataDir != "" {
			err = sess.SaveSession(matcher.cfg.SessionDataDir)
			if err != nil {
				matcher.log.Errorf("Error saving session %s: %v",
					sess.ID.String(), err)
			}
		}

		matcher.log.Noticef("Session %s successfully finished", sess.ID)
		matcher.removeSession(sess, nil)
	}

	return nil
}

func (matcher *Matcher) cancelSessionOnContextDone(ctx context.Context, sessPart *SessionParticipant) {
	go func() {
		<-ctx.Done()
		sess := sessPart.Session
		if (ctx.Err() != nil) && (!sess.Canceled) && (!sess.Done) {
			matcher.log.Warningf("Cancelling session %s due to context error on participant %s: %v", sess.ID, sessPart.ID, ctx.Err())
			matcher.cancelSessionChan <- cancelSessionChanReq{session: sessPart.Session, err: errors.Errorf("participant disconnected")}
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

func (matcher *Matcher) AddParticipant(ctx context.Context, maxAmount uint64, sessionName string) (*SessionParticipant, error) {
	if maxAmount < matcher.cfg.MinAmount {
		return nil, errors.Errorf("participation amount (%s) less than "+
			"minimum required (%s)", dcrutil.Amount(maxAmount),
			dcrutil.Amount(matcher.cfg.MinAmount))
	}

	curStakeDiffChangeDist := splitticket.StakeDiffChangeDistance(
		matcher.cfg.NetworkProvider.CurrentBlockHeight(), matcher.cfg.ChainParams)
	if curStakeDiffChangeDist < matcher.cfg.StakeDiffChangeStopWindow {
		return nil, errors.Errorf("current block too close to change of "+
			"stake difficulty (%d blocks)", curStakeDiffChangeDist)
	}

	if !matcher.cfg.NetworkProvider.ConnectedToDecredNetwork() {
		return nil, errors.Errorf("matcher is not currently connected to the" +
			"decred network")
	}

	req := addParticipantRequest{
		ctx:         ctx,
		maxAmount:   maxAmount,
		sessionName: sessionName,
		resp:        make(chan addParticipantResponse),
	}
	matcher.addParticipantRequests <- req

	resp := <-req.resp
	return resp.participant, resp.err
}

func (matcher *Matcher) WatchWaitingList(ctx context.Context, watcher chan []WaitingQueue) {
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
func (matcher *Matcher) SetParticipantsOutputs(ctx context.Context,
	sessionID ParticipantID, voteAddress, poolAddress, commitAddress,
	splitTxAddress dcrutil.Address, splitTxChange *wire.TxOut,
	splitTxOutPoints []*wire.OutPoint,
	secretNbHash splitticket.SecretNumberHash) (*wire.MsgTx, *wire.MsgTx,
	[]*ParticipantTicketOutput, uint32, error) {

	if voteAddress == nil {
		return nil, nil, nil, 0, errors.New("empty vote address")
	}

	if poolAddress == nil {
		return nil, nil, nil, 0, errors.New("empty pool address")
	}

	if commitAddress == nil {
		return nil, nil, nil, 0, errors.New("empty commitment address")
	}

	if splitTxAddress == nil {
		return nil, nil, nil, 0, errors.New("empty split tx address")
	}

	if len(splitTxOutPoints) == 0 {
		return nil, nil, nil, 0, errors.New("no outpoints to fund " +
			"split tx provided")
	}

	// TODO: check if number of split inputs is < than a certain threshold
	// (maybe 5 per part?)

	req := setParticipantOutputsRequest{
		ctx:              ctx,
		sessionID:        sessionID,
		voteAddress:      voteAddress,
		poolAddress:      poolAddress,
		commitAddress:    commitAddress,
		splitTxAddress:   splitTxAddress,
		splitTxChange:    splitTxChange,
		splitTxOutPoints: splitTxOutPoints,
		secretHash:       secretNbHash,
		resp:             make(chan setParticipantOutputsResponse),
	}

	matcher.setParticipantOutputsRequests <- req
	resp := <-req.resp
	return resp.splitTx, resp.ticket, resp.participants, resp.index, resp.err
}

func (matcher *Matcher) FundTicket(ctx context.Context, sessionID ParticipantID,
	inputsScriptSig [][]byte, revocationScriptSig []byte) ([][]byte, [][]byte, error) {

	if revocationScriptSig == nil {
		return nil, nil, errors.Errorf("empty revocationScriptSig")
	}

	req := fundTicketRequest{
		ctx:                   ctx,
		sessionID:             sessionID,
		ticketsInputScriptSig: inputsScriptSig,
		revocationScriptSig:   revocationScriptSig,
		resp:                  make(chan fundTicketResponse),
	}

	matcher.fundTicketRequests <- req
	resp := <-req.resp
	return resp.tickets, resp.revocations, resp.err
}

func (matcher *Matcher) FundSplit(ctx context.Context, sessionID ParticipantID,
	inputScriptSigs [][]byte, secretNb splitticket.SecretNumber) ([]byte,
	[]splitticket.SecretNumber, error) {

	req := fundSplitTxRequest{
		ctx:             ctx,
		sessionID:       sessionID,
		inputScriptSigs: inputScriptSigs,
		secretNb:        secretNb,
		resp:            make(chan fundSplitTxResponse),
	}
	matcher.fundSplitTxRequests <- req
	resp := <-req.resp
	return resp.splitTx, resp.secrets, resp.err

}
