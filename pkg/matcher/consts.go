package matcher

import (
	"math"

	"github.com/decred/dcrd/dcrutil"
)

const (
	// CommitmentLimits is the limit used in ticket commitments
	CommitmentLimits = uint16(0x5800)

	// RevocationFeeRate is the fee rate in Atoms/KB of the revocation tx.
	// 1e5 = 0.001 DCR
	RevocationFeeRate = int64(1e5)

	// MaximumExpiry accepted for split and ticket transactions
	MaximumExpiry = 16

	// VoterLotteryCommitmentScriptSize is the size of the pkscript of the
	// voter lottery commiment output (output 0 of the split tx). It is
	// the size of an OP_RETURN + 32 byte hash.
	VoterLotteryCommitmentScriptSize = 33

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
	// EmptySStxChangeAddr is a pre-calculated pkscript for use in SStx change
	// outputs that pays to a zeroed address. This is usually used in change
	// addresses that have zero value.
	EmptySStxChangeAddr = []byte{
		0xbd, 0x76, 0xa9, 0x14, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x88, 0xac}
)

// SessionParticipantFee returns the fee that a single participant of a ticket
// split tx with the given number of participants should pay
func SessionParticipantFee(numParticipants int) dcrutil.Amount {
	txSize := TicketTxInitialSize + numParticipants*TicketParticipantSize
	txSize += TicketParticipantSize // Pool input/outputs
	ticketFee, _ := dcrutil.NewAmount(float64(txSize) * TicketFeeEstimate)
	ticketFee = ticketFee + 50001 // just to increase chances of ticket being mined soon
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
