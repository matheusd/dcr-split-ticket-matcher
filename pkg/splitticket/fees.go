package splitticket

import (
	"math"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrwallet/wallet/txrules"
	"github.com/pkg/errors"
)

const (
	// TicketTxInitialSize is the initial size estimate for the ticket
	// transaction. It includes the tx header + the txout for the ticket
	// voting address
	TicketTxInitialSize = 12 + // tx header prefix (sertype, version, locktime, expiry)
		1 + 1 + 1 + // witness varint + input count varint + output count varint
		8 + 2 + 1 + 26 // ticket submission TxOut = amount + version + script

	// TicketParticipantSize is the size estimate for each additional
	// participant in a split ticket purchase (the txIn + 2 txOuts)
	TicketParticipantSize = 32 + 4 + 1 + 4 + // TxIn NonWitness = Outpoint hash + Index + tree + Sequence
		8 + 4 + 4 + // TxIn Witness = Amount + Block Height + Block Index
		1 + 108 + // TxIn len(ScriptSig) + ScriptSig
		8 + 2 + 1 + 32 + // Stake Commitment TxOut = amount + version + script
		8 + 2 + 1 + 26 // Stake Change TxOut = amount + version + script

	// TicketFeeEstimate is the fee rate estimate (in dcr/byte) of the fee in
	// a ticket purchase
	TicketFeeEstimate float64 = 0.001 / 1000
)

// TicketSizeEstimate returns the size estimate for the ticket transaction for
// the given number of participants
func TicketSizeEstimate(numParticipants int) int {
	txSize := TicketTxInitialSize + numParticipants*TicketParticipantSize
	txSize += TicketParticipantSize // Pool input/outputs
	return txSize
}

// SessionParticipantFee returns the fee that a single participant of a ticket
// split tx with the given number of participants should pay
func SessionParticipantFee(numParticipants int) dcrutil.Amount {
	txSize := TicketSizeEstimate(numParticipants)
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

// SessionPoolFee returns the estimate for pool fee contribution for a split
// ticket session, given the parameters.
func SessionPoolFee(numParticipants int, ticketPrice dcrutil.Amount,
	blockHeight int, poolFeePerc float64, net *chaincfg.Params) dcrutil.Amount {

	ticketTxFee := SessionFeeEstimate(numParticipants)
	minPoolFee := txrules.StakePoolTicketFee(ticketPrice, ticketTxFee, int32(blockHeight),
		poolFeePerc, net)
	return minPoolFee
}

// CheckParticipantSessionPoolFee checks whether the pool fee paid by a given
// participant is fair, assuming a given ticket price, poolFeeRate and
// contribution amount.
func CheckParticipantSessionPoolFee(numParticipants int, ticketPrice dcrutil.Amount,
	contribAmount, partPoolFee, partFee dcrutil.Amount, blockHeight int,
	poolFeePerc float64, net *chaincfg.Params) error {

	poolFee := SessionPoolFee(numParticipants, ticketPrice, blockHeight,
		poolFeePerc, net)
	contribPerc := float64(contribAmount+partFee) / float64(ticketPrice-poolFee)
	partPoolFeePerc := float64(partPoolFee) / float64(poolFee)
	if partPoolFeePerc-contribPerc > 0.0001 {
		return errors.Errorf("participant pool fee contribution (%f%%) higher "+
			"then ticket contribution (%f%%)", partPoolFeePerc, contribPerc)
	}

	return nil
}
