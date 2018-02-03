package matcher

import (
	"fmt"
	"math"

	"github.com/decred/dcrd/dcrutil"
)

const (
	// TicketTxInitialSize is the initial size estimate for the ticket
	// transaction. It includes the tx header + the txout for the ticket
	// voting address
	TicketTxInitialSize = 2 + 2 + 4 + 4 + 2 + // tx header = SerType + Version + LockTime + Expiry + I/O count
		8 + 2 + 24 // ticket submission TxOut = amount + version + script

	// TicketParticipantSize is the size estimate for each additional
	// participant in a split ticket purchase (the txIn + 2 txOuts)
	TicketParticipantSize = 32 + 4 + 1 + 4 + // TxIn NonWitness = Outpoint hash + Index + tree + Sequence
		8 + 4 + 4 + // TxIn Witness = Amount + Block Height + Block Index
		106 + // TxIn ScriptSig
		8 + 2 + 32 + // Stake Commitment TxOut = amount + version + script
		8 + 2 + 26 + 8 // Stake Change TxOut = amount + version + script + commit amount

	// TicketFeeEstimate is the fee rate estimate (in dcr/byte) of the fee in
	// a ticket purchase
	TicketFeeEstimate float64 = 0.001 / 1000

	// ParticipantFeeOverheadEstimate is how much more than what is strictly
	// necessary for the ticket purchase each participant should have to
	// purchase the tickets
	ParticipantFeeOverheadEstimate dcrutil.Amount = 2 * 1e5
)

var (
	ErrLowAmount           = fmt.Errorf("Amount too low to participate in ticket purchase")
	ErrTooManyParticipants = fmt.Errorf("Too many online participants at the moment")
)

// SessionParticipantFee returns the fee that a single participant of a ticket
// split tx with the given number of participants should pay
func SessionParticipantFee(numParticipants int) dcrutil.Amount {
	txSize := TicketTxInitialSize + numParticipants*TicketParticipantSize
	ticketFee, _ := dcrutil.NewAmount(float64(txSize) * TicketFeeEstimate)
	partFee := dcrutil.Amount(math.Ceil(float64(ticketFee) / float64(numParticipants)))
	return partFee
}

// SessionFeeEstimate returns an estimate for the fees of a session with the
// given number of participants.
//
// Note that the calculation is done from SessionParticipantFee in order to be
// certain that all participants will pay an integer and equal amount of fees.
func SessionFeeEstimate(numParticipants int) dcrutil.Amount {
	return SessionParticipantFee(numParticipants) * dcrutil.Amount(numParticipants)
}
