package matcher

import (
	"fmt"
	"time"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"
	"github.com/pkg/errors"
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
	ID                ParticipantID
	CommitAmount      dcrutil.Amount
	Fee               dcrutil.Amount
	PoolFee           dcrutil.Amount
	VoteAddress       dcrutil.Address
	PoolAddress       dcrutil.Address
	CommitmentAddress dcrutil.Address
	SplitTxAddress    dcrutil.Address
	SecretHash        splitticket.SecretNumberHash
	SecretNb          splitticket.SecretNumber

	Session *Session
	Index   int

	votePkScript          []byte
	poolPkScript          []byte
	commitmentPkScript    []byte
	splitPkScript         []byte
	splitTxChange         *wire.TxOut
	splitTxInputs         []*wire.TxIn
	ticketsScriptSig      [][]byte
	revocationScriptSig   []byte
	poolFeeInputScriptSig []byte
	splitTxUtxos          splitticket.UtxoMap

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

// createIOs creates the inputs and outputs for the ticket and split
// transactions, according to the currently stored addresses.
func (part *SessionParticipant) createIOs() error {
	voteScript, err := txscript.PayToSStx(part.VoteAddress)
	if err != nil {
		return errors.Wrapf(err, "error creating ticket vote output script")
	}

	poolScript, err := txscript.GenerateSStxAddrPush(part.PoolAddress,
		part.Session.PoolFee, CommitmentLimits)
	if err != nil {
		return errors.Wrapf(err, "error creating ticket pool output script")
	}

	commitScript, err := txscript.GenerateSStxAddrPush(part.CommitmentAddress,
		part.CommitAmount+part.Fee, CommitmentLimits)
	if err != nil {
		return errors.Wrapf(err, "error creating ticket commitment output script")
	}

	splitScript, err := txscript.PayToAddrScript(part.SplitTxAddress)
	if err != nil {
		return errors.Wrapf(err, "error creating split output script")
	}

	part.votePkScript = voteScript
	part.poolPkScript = poolScript
	part.commitmentPkScript = commitScript
	part.splitPkScript = splitScript
	return nil
}

func (part *SessionParticipant) replaceTicketIOs(ticket *wire.MsgTx) {
	ticket.TxOut[0].PkScript = part.votePkScript
	ticket.TxOut[1].PkScript = part.poolPkScript

	ticket.TxIn[0].SignatureScript = part.poolFeeInputScriptSig

	// replace the scriptsig for the input of each participant into the ticket
	for i, p := range part.Session.Participants {
		if len(p.ticketsScriptSig) > part.Index {
			// do note the following: it's TxIn[i+1] because the first TxIn of the
			// ticket is the pool fee (which is already signed by the matcher). Also,
			// we need the corresponding script for the given participant, given
			// that the selected voter participant is `part`, therefore we grab
			// the signature `p` created for the `part.Index` ticket.
			ticket.TxIn[i+1].SignatureScript = p.ticketsScriptSig[part.Index]
		}
	}
}

func (part *SessionParticipant) replaceRevocationInput(ticket, revocation *wire.MsgTx) {
	ticketHash := ticket.TxHash()
	revocation.TxIn[0].SignatureScript = part.revocationScriptSig
	revocation.TxIn[0].PreviousOutPoint.Hash = ticketHash
}

// SessionID stores the unique id for an in-progress ticket buying session
type SessionID uint16

func (id SessionID) String() string {
	return fmt.Sprintf("%.4x", uint16(id))
}

// Session is a particular ticket being built
type Session struct {
	ID              SessionID
	Participants    []*SessionParticipant
	TicketPrice     dcrutil.Amount
	MainchainHash   chainhash.Hash
	MainchainHeight uint32
	VoterIndex      int
	PoolFee         dcrutil.Amount
	ChainParams     *chaincfg.Params
	SplitTxPoolOut  *wire.TxOut
	TicketPoolIn    *wire.TxIn
	StartTime       time.Time
	Done            bool
	Canceled        bool
	TicketExpiry    uint32
}

// AllOutputsFilled returns true if all commitment and change outputs for all
// participants have been filled
func (sess *Session) AllOutputsFilled() bool {
	for _, p := range sess.Participants {
		if (p.VoteAddress == nil) ||
			(p.PoolAddress == nil) ||
			(p.CommitmentAddress == nil) ||
			(p.SplitTxAddress == nil) ||
			len(p.splitTxInputs) == 0 {

			return false
		}
	}
	return true
}

// TicketIsFunded returns true if all ticket inputs have been filled.
func (sess *Session) TicketIsFunded() bool {
	for _, p := range sess.Participants {
		if len(p.ticketsScriptSig) != len(sess.Participants) {
			return false
		}
	}
	return true
}

// SplitTxIsFunded returns true if the split tx is funded
func (sess *Session) SplitTxIsFunded() bool {
	for _, p := range sess.Participants {
		for _, in := range p.splitTxInputs {
			if len(in.SignatureScript) == 0 {
				return false
			}
		}
	}
	return true
}

func (sess *Session) addCommonTicketOutputs(ticket *wire.MsgTx) error {

	zeroed := [20]byte{}
	addrZeroed, err := dcrutil.NewAddressPubKeyHash(zeroed[:], sess.ChainParams, 0)
	if err != nil {
		return err
	}

	voteScript, err := txscript.PayToSStx(addrZeroed)
	if err != nil {
		return err
	}

	limits := uint16(0x5800)
	commitScript, err := txscript.GenerateSStxAddrPush(addrZeroed, sess.PoolFee, limits)
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

// addVoterSelectionData creates the OP_RETURN output on the split tx with the
// voter hash to allow for fraud detection
func (sess *Session) addVoterSelectionData(split *wire.MsgTx) {
	hashes := make([]splitticket.SecretNumberHash, len(sess.Participants))
	for i, p := range sess.Participants {
		hashes[i] = p.SecretHash
	}
	hash := splitticket.SecretNumberHashesHash(hashes, &sess.MainchainHash)

	b := txscript.NewScriptBuilder()
	b.
		AddOp(txscript.OP_RETURN).
		AddData(hash[:])

	script, err := b.Script()
	if err != nil {
		panic(err) // TODO: handle this bettter
	}

	out := wire.NewTxOut(0, script)
	split.AddTxOut(out)
}

// CreateTransactions creates the ticket and split tx transactions with all the
// currently available information
func (sess *Session) CreateTransactions() (*wire.MsgTx, *wire.MsgTx, error) {
	var spOutIndex uint32

	ticket := wire.NewMsgTx()
	splitTx := wire.NewMsgTx()

	ticket.Expiry = sess.TicketExpiry

	sess.addCommonTicketOutputs(ticket)
	sess.addVoterSelectionData(splitTx)
	spOutIndex++

	// this is the output that will correspond to the pool fee commitment
	splitTx.AddTxOut(sess.SplitTxPoolOut)
	spOutIndex++

	for _, p := range sess.Participants {
		ticket.AddTxIn(wire.NewTxIn(&wire.OutPoint{Index: spOutIndex}, nil))

		ticket.AddTxOut(wire.NewTxOut(0, p.commitmentPkScript))
		ticket.AddTxOut(wire.NewTxOut(0, EmptySStxChangeAddr))

		splitTx.AddTxOut(wire.NewTxOut(int64(p.CommitAmount+p.Fee),
			p.splitPkScript))
		spOutIndex++

		if p.splitTxChange != nil {
			splitTx.AddTxOut(&wire.TxOut{p.splitTxChange.Value,
				p.splitTxChange.Version, p.splitTxChange.PkScript})
			spOutIndex++
		}

		for _, in := range p.splitTxInputs {
			outp := &wire.OutPoint{in.PreviousOutPoint.Hash,
				in.PreviousOutPoint.Index, in.PreviousOutPoint.Tree}
			splitTx.AddTxIn(wire.NewTxIn(outp, in.SignatureScript))
		}
	}

	// back-fill the ticket input's outpoints with the split tx hash
	splitHash := splitTx.TxHash()
	for _, in := range ticket.TxIn {
		in.PreviousOutPoint.Hash = splitHash
	}

	return ticket, splitTx, nil
}

// CreateVoterTransactions creates the transactions (including the revocation)
// once the voter is known to the session.
func (sess *Session) CreateVoterTransactions() (*wire.MsgTx, *wire.MsgTx, *wire.MsgTx, error) {
	if sess.VoterIndex < 0 {
		return nil, nil, nil, errors.Errorf("voter not yet known")
	}

	ticket, splitTx, err := sess.CreateTransactions()
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "error creating transaction templates")
	}

	voter := sess.Participants[sess.VoterIndex]

	voter.replaceTicketIOs(ticket)

	ticketHash := ticket.TxHash()

	revocation, err := CreateUnsignedRevocation(&ticketHash, ticket, dcrutil.Amount(1e5))
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "error creating unsigned revocation")
	}

	voter.replaceRevocationInput(ticket, revocation)

	return ticket, splitTx, revocation, nil
}

// ParticipantTicketOutputs returns an array with information on the outputs of
// of the ticket built for each voting participant. Only safe to be called after
// all participants have sent their outputs.
func (sess *Session) ParticipantTicketOutputs() []*ParticipantTicketOutput {
	res := make([]*ParticipantTicketOutput, len(sess.Participants))
	for i, p := range sess.Participants {
		res[i] = &ParticipantTicketOutput{
			Amount:       p.CommitAmount,
			SecretHash:   &p.SecretHash,
			PoolPkScript: p.poolPkScript,
			VotePkScript: p.votePkScript,
		}
	}
	return res
}

// SecretNumbers returns an array of the secret number of all participants
func (sess *Session) SecretNumbers() splitticket.SecretNumbers {
	res := splitticket.SecretNumbers(make([]splitticket.SecretNumber, len(sess.Participants)))
	for i, p := range sess.Participants {
		res[i] = p.SecretNb
	}
	return res
}

// FindVoterIndex finds the index of the voter, given the current setup of
// the session.
func (sess *Session) FindVoterIndex() int {
	var totalCommitment, sum uint64
	for _, p := range sess.Participants {
		totalCommitment += uint64(p.CommitAmount)
	}

	nbs := sess.SecretNumbers()
	nbsHash := nbs.Hash(&sess.MainchainHash)
	coinIdx := nbsHash.SelectedCoin(totalCommitment)
	for i, p := range sess.Participants {
		sum += uint64(p.CommitAmount)
		if coinIdx < sum {
			return i
		}
	}

	panic("Should never get here")
}

// SplitUtxoMap returns the full utxo map for the split transaction's inputs
// for every participant.
// Returns an error if the same utxo is used more than once across all
// participants.
func (sess *Session) SplitUtxoMap() (splitticket.UtxoMap, error) {
	res := make(splitticket.UtxoMap)
	for _, p := range sess.Participants {
		for o, u := range p.splitTxUtxos {
			if _, has := res[o]; has {
				return nil, errors.Errorf("output %s is present multiple "+
					"times in the split ticket session", o)
			}
			res[o] = u
		}
	}

	return res, nil
}

// SecretNumberHashes returns an array with the individual secret number hashes
// for all participants
func (sess *Session) SecretNumberHashes() []splitticket.SecretNumberHash {
	res := make([]splitticket.SecretNumberHash, len(sess.Participants))
	for i, p := range sess.Participants {
		res[i] = p.SecretHash
	}
	return res
}

// ParticipantAmounts returns an array with the commitment amounts for each
// individual participant
func (sess *Session) ParticipantAmounts() []dcrutil.Amount {
	res := make([]dcrutil.Amount, len(sess.Participants))
	for i, p := range sess.Participants {
		res[i] = p.CommitAmount
	}
	return res
}