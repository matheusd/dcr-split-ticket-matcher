package matcher

import (
	"math"

	"github.com/decred/dcrd/dcrutil"
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
	ID              SessionID
	Amount          dcrutil.Amount
	Fee             dcrutil.Amount
	CommitmentTxOut *wire.TxOut
	ChangeTxOut     *wire.TxOut
	Input           *wire.TxIn
	SplitTx         *wire.MsgTx
	Session         *Session
	Index           int

	matcherErrorChan chan error
}

// SessionID stores the unique id for an in-progress ticket buying session
type SessionID uint64

// Session is a particular ticket being built
type Session struct {
	Participants []*SessionParticipant
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

	addParticipantRequests chan addParticipantRequest
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

func (matcher *Matcher) newSessionID() SessionID {
	// TODO: rw lock matcher.sessions here
	id := MustRandUint64()
	for _, has := matcher.sessions[SessionID(id)]; has; {
		id = MustRandUint64()
	}
	return SessionID(id)
}
