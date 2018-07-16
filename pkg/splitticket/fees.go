package splitticket

import (
	"math"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrwallet/wallet/txrules"
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

// SessionParticipantPoolFee returns the estimate for pool fee contribution for
// each participant of a split ticket, given the parameters. Note that this
// assumes each participant will pay the same amount, to add up to the total
// pool fee.
func SessionParticipantPoolFee(numParticipants int, ticketPrice dcrutil.Amount,
	blockHeight int, poolFeePerc float64, net *chaincfg.Params) dcrutil.Amount {

	ticketTxFee := SessionFeeEstimate(numParticipants)
	minPoolFee := txrules.StakePoolTicketFee(ticketPrice, ticketTxFee, int32(blockHeight),
		poolFeePerc, net)
	poolFeePart := dcrutil.Amount(math.Ceil(float64(minPoolFee) / float64(numParticipants)))
	return poolFeePart
}
