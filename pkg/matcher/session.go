package matcher

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
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
		part.Session.PoolFee, splitticket.CommitmentLimits)
	if err != nil {
		return errors.Wrapf(err, "error creating ticket pool output script")
	}

	commitScript, err := txscript.GenerateSStxAddrPush(part.CommitmentAddress,
		part.CommitAmount+part.Fee, splitticket.CommitmentLimits)
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
	SelectedCoin    dcrutil.Amount
	PoolFee         dcrutil.Amount
	TicketFee       dcrutil.Amount
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
func (sess *Session) addVoterSelectionData(split *wire.MsgTx) error {
	hashes := make([]splitticket.SecretNumberHash, len(sess.Participants))
	for i, p := range sess.Participants {
		hashes[i] = p.SecretHash
	}

	hash := splitticket.CalcLotteryCommitmentHash(sess.SecretNumberHashes(),
		sess.ParticipantAmounts(), sess.VoteAddresses(), &sess.MainchainHash)

	b := txscript.NewScriptBuilder()
	b.
		AddOp(txscript.OP_RETURN).
		AddData(hash[:])

	script, err := b.Script()
	if err != nil {
		return errors.Wrapf(err, "error creating OP_RETURN script")
	}

	out := wire.NewTxOut(0, script)
	split.AddTxOut(out)
	return nil
}

// CreateTransactions creates the ticket and split tx transactions with all the
// currently available information
func (sess *Session) CreateTransactions() (*wire.MsgTx, *wire.MsgTx, error) {
	var spOutIndex uint32

	ticket := wire.NewMsgTx()
	splitTx := wire.NewMsgTx()

	ticket.Expiry = sess.TicketExpiry

	err := sess.addCommonTicketOutputs(ticket)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error adding common ticket outputs")
	}

	err = sess.addVoterSelectionData(splitTx)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error adding voter selection data to split tx")
	}

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
			splitTx.AddTxOut(&wire.TxOut{
				Value:    p.splitTxChange.Value,
				Version:  p.splitTxChange.Version,
				PkScript: p.splitTxChange.PkScript,
			})
			spOutIndex++
		}

		for _, in := range p.splitTxInputs {
			outp := &wire.OutPoint{
				Hash:  in.PreviousOutPoint.Hash,
				Index: in.PreviousOutPoint.Index,
				Tree:  in.PreviousOutPoint.Tree,
			}
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

	revocation, err := splitticket.CreateUnsignedRevocation(&ticketHash, ticket, dcrutil.Amount(1e5))
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

// SecretNumbers returns a slice with the secret number of all participants
func (sess *Session) SecretNumbers() []splitticket.SecretNumber {
	res := make([]splitticket.SecretNumber, len(sess.Participants))
	for i, p := range sess.Participants {
		res[i] = p.SecretNb
	}
	return res
}

// FindVoterCoinIndex returns the coin and index of the voter for the current
// session. Assumes all secret numbers are known.
func (sess *Session) FindVoterCoinIndex() (dcrutil.Amount, int) {
	return splitticket.CalcLotteryResult(sess.SecretNumbers(),
		sess.ParticipantAmounts(), &sess.MainchainHash)
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

// VoteAddresses returns a slice with every participant's vote addresses in
// order.
func (sess *Session) VoteAddresses() []dcrutil.Address {
	res := make([]dcrutil.Address, len(sess.Participants))
	for i, p := range sess.Participants {
		res[i] = p.VoteAddress
	}
	return res
}

// SaveSession saves the session data as a text file in the given directory.
// The name of the file will be the ticket hash.
func (sess *Session) SaveSession(sessionDir string) error {

	_, err := os.Stat(sessionDir)

	if os.IsNotExist(err) {
		err = os.MkdirAll(sessionDir, 0700)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	ticket, split, revocation, err := sess.CreateVoterTransactions()
	if err != nil {
		return errors.Wrapf(err, "error creating voter txs")
	}

	ticketTempl, _, err := sess.CreateTransactions()
	if err != nil {
		return errors.Wrapf(err, "error creating template txs")
	}

	ticketHashHex := ticket.TxHash().String()
	ticketBytes, err := ticket.Bytes()
	if err != nil {
		return err
	}

	fname := filepath.Join(sessionDir, ticketHashHex)

	fflags := os.O_TRUNC | os.O_CREATE | os.O_WRONLY
	f, err := os.OpenFile(fname, fflags, 0600)
	if err != nil {
		return errors.Wrapf(err, "error opening file '%s'")
	}
	w := bufio.NewWriter(f)
	hexWriter := hex.NewEncoder(w)

	defer func() {
		w.Flush()
		f.Sync()
		f.Close()
	}()

	splitHash := split.TxHash()
	splitBytes, err := split.Bytes()
	if err != nil {
		return err
	}

	revocationHash := revocation.TxHash()
	revocationBytes, err := revocation.Bytes()
	if err != nil {
		return err
	}

	utxos, err := sess.SplitUtxoMap()
	if err != nil {
		return errors.Wrapf(err, "error getting split utxo map")
	}

	out := func(format string, args ...interface{}) {
		w.WriteString(fmt.Sprintf(format, args...))
	}

	splitUtxos, err := sess.SplitUtxoMap()
	if err != nil {
		return errors.Wrapf(err, "error getting split utxos")
	}

	actualSplitFee, err := splitticket.FindTxFee(split, splitUtxos)
	if err != nil {
		return errors.Wrapf(err, "error calculating actual split fee")
	}
	actualSplitFeeRate := float64(actualSplitFee) / float64(len(splitBytes)*1e5)

	actualTicketFee, err := splitticket.FindTicketTxFee(split, ticket)
	if err != nil {
		return errors.Wrapf(err, "error calculating actual ticket fee")
	}
	actualTicketFeeRate := float64(actualTicketFee) / float64(len(ticketBytes)*1e5)

	actualRevocationFee, err := splitticket.FindRevocationTxFee(ticket, revocation)
	if err != nil {
		return errors.Wrapf(err, "error calculating actual revocation fee")
	}
	actualRevocationFeeRate := float64(actualRevocationFee) / float64(len(revocationBytes)*1e5)

	out("====== General Info ======\n")

	out("Session ID = %s\n", sess.ID)
	out("Stt Time = %s\n", sess.StartTime.String())
	out("End Time = %s\n", time.Now().String())
	out("Mainchain Hash = %s\n", sess.MainchainHash.String())
	out("Mainchain Height = %d\n", sess.MainchainHeight)
	out("Ticket Price = %s\n", sess.TicketPrice)
	out("Number of Participants = %d\n", len(sess.Participants))
	out("Split tx Fee = %s (%.4f DCR/KB)\n", actualSplitFee, actualSplitFeeRate)
	out("Estimated Ticket Fee = %s\n", sess.TicketFee)
	out("Actual Ticket Fee = %s (%.4f DCR/KB)\n", actualTicketFee, actualTicketFeeRate)
	out("Revocation Fee = %s (%.4f DCR/KB)\n", actualRevocationFee, actualRevocationFeeRate)
	out("Pool Fee = %s\n", sess.PoolFee)
	out("Split Transaction hash = %s\n", splitHash.String())
	out("Final Ticket Hash = %s\n", ticketHashHex)
	out("Final Revocation Hash = %s\n", revocationHash.String())

	out("\n")
	out("====== Voter Selection ======\n")

	commitHash := splitticket.CalcLotteryCommitmentHash(sess.SecretNumberHashes(),
		sess.ParticipantAmounts(), sess.VoteAddresses(), &sess.MainchainHash)

	out("Participant Amounts = %v\n", sess.ParticipantAmounts())
	out("Vote Addresses = %v\n", encodedAddresses(sess.VoteAddresses()))
	out("Secret Hashes = %v\n", sess.SecretNumberHashes())
	out("Voter Lottery Commitment Hash = %s\n", hex.EncodeToString(commitHash[:]))
	out("Secret Numbers = %v\n", sess.SecretNumbers())
	out("Selected Coin = %s\n", sess.SelectedCoin)
	out("Selected Voter Index = %d\n", sess.VoterIndex)

	out("\n")
	out("====== Final Transactions ======\n")

	out("== Split Transaction ==\n")
	hexWriter.Write(splitBytes)
	out("\n\n")

	out("== Ticket ==\n")
	hexWriter.Write(ticketBytes)
	out("\n\n")

	out("== Revocation ==\n")
	hexWriter.Write(revocationBytes)
	out("\n\n")

	out("\n")
	out("====== Split Inputs ======\n")

	for outp, entry := range utxos {
		out("Input %s = %s\n", outp, entry.Value)
	}

	out("\n")
	out("====== Participant Intermediate Information ======\n")
	for i, p := range sess.Participants {
		p.replaceTicketIOs(ticketTempl)
		ticketHash := ticketTempl.TxHash()
		revocationTempl, err := splitticket.CreateUnsignedRevocation(&ticketHash, ticketTempl,
			splitticket.RevocationFeeRate)
		if err != nil {
			return errors.Wrapf(err, "error creating unsigned revocation")
		}

		p.replaceRevocationInput(ticketTempl, revocationTempl)

		voteScript := hex.EncodeToString(p.votePkScript)
		poolScript := hex.EncodeToString(p.poolPkScript)

		partTicket, err := ticketTempl.Bytes()
		if err != nil {
			return errors.Wrapf(err, "error encoding participant %d ticket", i)
		}

		partRevocation, err := revocationTempl.Bytes()
		if err != nil {
			return errors.Wrapf(err, "error encoding participant %d revocation", i)
		}

		out("\n")
		out("== Participant %d ==\n", i)
		out("Amount = %s\n", p.CommitAmount)
		out("Change = %s\n", dcrutil.Amount(p.splitTxChange.Value))
		out("Secret Hash = %s\n", p.SecretHash)
		out("Secret Number = %d\n", p.SecretNb)
		out("Vote Address = %s\n", p.VoteAddress.EncodeAddress())
		out("Pool Address = %s\n", p.PoolAddress.EncodeAddress())
		out("Vote PkScript = %s\n", voteScript)
		out("Pool PkScript = %s\n", poolScript)
		out("Ticket = %s\n", hex.EncodeToString(partTicket))
		out("Revocation = %s\n", hex.EncodeToString(partRevocation))
	}

	return nil
}
