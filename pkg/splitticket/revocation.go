package splitticket

import (
	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrwallet/wallet/txrules"
	"github.com/pkg/errors"
)

const (
	// RedeemPoolVotingScriptSize is the maximum size of a scriptSig used to redeem
	// a ticket tx (via vote or revocation) when using a stakepool.
	RedeemPoolVotingScriptSize = 1 + 73 + 1 + 73
)

// RevocationFeeRate is the fee rate in Atoms/KB of the revocation tx for a
// given network.
func RevocationFeeRate(params *chaincfg.Params) dcrutil.Amount {
	if params.Name == "simnet" {
		// due to very low ticket prices in simnet, we need to use a very small
		// revocation. This shouldn't be a problem since in simnet the
		// revocation should be mined anyway.
		return 1e4
	}

	return TxFeeRate
}

// CheckRevocation checks whether the revocation for the given ticket respects
// the rules for split ticket buying. The ticket must have passed the
// CheckTicket function.
func CheckRevocation(ticket, revocation *wire.MsgTx, params *chaincfg.Params) error {
	// ensure this looks like a decred transaction
	err := blockchain.CheckTransactionSanity(revocation, params)
	if err != nil {
		return errors.Wrap(err, "revocation failed sanity check")
	}

	// ensure this actually validates as a revocation
	err = stake.CheckSSRtx(revocation)
	if err != nil {
		return errors.Wrap(err, "revocation tx is not an rrtx")
	}

	// ensure the various tx locks don't prevent the revocation from being mined
	if revocation.Expiry != 0 {
		return errors.Errorf("revocation has expiry (%d) different than zero",
			revocation.Expiry)
	}
	if revocation.LockTime != 0 {
		return errors.Errorf("revocation has locktime (%d) different than zero",
			revocation.LockTime)
	}
	if revocation.TxIn[0].Sequence != wire.MaxTxInSequenceNum {
		return errors.Errorf("sequence number of revocation input (%d) "+
			"different than expected (%d)", revocation.TxIn[0].Sequence,
			wire.MaxTxInSequenceNum)
	}
	if revocation.Version != wire.TxVersion {
		return errors.Errorf("revocation txversion (%d) different than "+
			"expected (%d)", revocation.Version, wire.TxVersion)
	}

	for i, out := range revocation.TxOut {
		if out.Version != txscript.DefaultScriptVersion {
			return errors.Errorf("output %d of revocation does not use the "+
				"default script version (%d)", i, out.Version)
		}
	}

	// ensure the revocation scriptSig successfully signs the ticket output
	engine, err := txscript.NewEngine(ticket.TxOut[0].PkScript, revocation, 0,
		currentScriptFlags, ticket.TxOut[0].Version, nil)
	if err != nil {
		return errors.Wrap(err, "error creating script engine")
	}
	err = engine.Execute()
	if err != nil {
		return errors.Wrap(err, "error verifying revocation scriptSig")
	}

	sstxPayTypes, sstxPkhs, sstxAmts, _, sstxRules, sstxLimits :=
		stake.TxSStxStakeOutputInfo(ticket)

	ssrtxPayTypes, ssrtxPkhs, ssrtxAmts, err :=
		stake.TxSSRtxStakeOutputInfo(revocation, params)
	if err != nil {
		return errors.Wrapf(err, "error decoding outputs for revocation")
	}

	// Quick check to make sure the number of SStx outputs is equal
	// to the number of SSGen outputs.
	if (len(sstxPkhs) != len(ssrtxPkhs)) || (len(sstxAmts) != len(ssrtxAmts)) {
		return errors.Errorf("incongruent payee count between ticket and " +
			"revocation")
	}

	// calculate the resulting amounts with a zero payout (rrtx)
	ssrtxCalcAmts := stake.CalculateRewards(sstxAmts, ticket.TxOut[0].Value, 0)

	// ensure the returned amounts actually correspond to the expected
	// calculated amounts
	err = stake.VerifyStakingPkhsAndAmounts(sstxPayTypes, sstxPkhs,
		ssrtxAmts, ssrtxPayTypes, ssrtxPkhs, ssrtxCalcAmts,
		false /* revocation */, sstxRules, sstxLimits)

	if err != nil {
		return errors.Wrapf(err, "consensus rule violation for revocation")
	}

	// ensure the tx fee for the revocation is >= default fee
	amountIn := dcrutil.Amount(ticket.TxOut[0].Value)
	amountOut := totalOutputAmount(revocation)
	if amountOut >= amountIn {
		return errors.Wrapf(err, "total output amount in revocation (%s) >= "+
			"total input amount in ticket (%s)", amountOut, amountIn)
	}
	fee := amountIn - amountOut
	serializedSize := int64(revocation.SerializeSize())
	feeRate := RevocationFeeRate(params)
	minFee := dcrutil.Amount((serializedSize * int64(feeRate)) / 1000)
	if fee < minFee {
		return errors.Errorf("revocation fee (%s) less than minimum required "+
			"amount (%s)", fee, minFee)
	}

	// ensure the tx fee for the revocation is not higher than an arbitrary
	// amount, to prevent fee drain.
	if fee > 2*minFee {
		return errors.Errorf("revocation fee (%s) higher than  2 times the "+
			"minimum required amount (%s)", fee, 2*minFee)
	}

	return nil
}

// FindRevocationTxFee finds the revocation transaction fee, assuming the ticket
// is correct. Only safe to be called on ticket and revocation transactions
// that have passed their respective check functions.
func FindRevocationTxFee(ticket, revocation *wire.MsgTx) (dcrutil.Amount, error) {
	if len(ticket.TxOut) == 0 {
		return 0, errors.Errorf("ticket does not have outputs")
	}

	out := ticket.TxOut[0]
	ticketUtxos := make(UtxoMap, 1)
	ticketHash := ticket.TxHash()
	outp := wire.OutPoint{
		Hash:  ticketHash,
		Index: 0,
		Tree:  wire.TxTreeStake,
	}
	ticketUtxos[outp] = UtxoEntry{
		PkScript: out.PkScript,
		Value:    dcrutil.Amount(out.Value),
		Version:  out.Version,
	}

	return FindTxFee(revocation, ticketUtxos)
}

// CreateUnsignedRevocation creates an unsigned revocation transaction that
// revokes a missed or expired ticket.  Revocations must carry a relay fee and
// this function can error if the revocation contains no suitable output to
// decrease the estimated relay fee from.
//
// This is based on the dcrwallet code, copied here due to it not being
// originally exported.
func CreateUnsignedRevocation(ticketHash *chainhash.Hash,
	ticketPurchase *wire.MsgTx, feePerKB dcrutil.Amount) (*wire.MsgTx, error) {
	// Parse the ticket purchase transaction to determine the required output
	// destinations for vote rewards or revocations.
	ticketPayKinds, ticketHash160s, ticketValues, _, _, _ :=
		stake.TxSStxStakeOutputInfo(ticketPurchase)

	// Calculate the output values for the revocation.  Revocations do not
	// contain any subsidy.
	revocationValues := stake.CalculateRewards(ticketValues,
		ticketPurchase.TxOut[0].Value, 0)

	// Begin constructing the revocation transaction.
	revocation := wire.NewMsgTx()

	// Revocations reference the ticket purchase with the first (and only)
	// input.
	ticketOutPoint := wire.NewOutPoint(ticketHash, 0, wire.TxTreeStake)
	revocation.AddTxIn(wire.NewTxIn(ticketOutPoint, wire.NullValueIn, nil))

	// All remaining outputs pay to the output destinations and amounts tagged
	// by the ticket purchase.
	for i, hash160 := range ticketHash160s {
		scriptFn := txscript.PayToSSRtxPKHDirect
		if ticketPayKinds[i] { // P2SH
			scriptFn = txscript.PayToSSRtxSHDirect
		}
		// Error is checking for a nil hash160, just ignore it.
		script, _ := scriptFn(hash160)
		revocation.AddTxOut(wire.NewTxOut(revocationValues[i], script))
	}

	// Calculate the estimated signed serialize size. Note that, since the
	// revocation is constructed unsiged, we add the maximum possible size for
	// a pool voting signature.
	sizeEstimate := revocation.SerializeSize() + RedeemPoolVotingScriptSize
	feeEstimate := txrules.FeeForSerializeSize(feePerKB, sizeEstimate)

	// Revocations must pay a fee but do so by decreasing one of the output
	// values instead of increasing the input value and using a change output.
	//
	// Reduce the output value of one of the outputs to accomodate for the relay
	// fee.  To avoid creating dust outputs, a suitable output value is reduced
	// by the fee estimate only if it is large enough to not create dust.  This
	// code does not currently handle reducing the output values of multiple
	// commitment outputs to accomodate for the fee.
	for _, output := range revocation.TxOut {
		if dcrutil.Amount(output.Value) > feeEstimate {
			amount := dcrutil.Amount(output.Value) - feeEstimate
			if !txrules.IsDustAmount(amount, len(output.PkScript), feePerKB) {
				output.Value = int64(amount)
				return revocation, nil
			}
		}
	}
	return nil, errors.New("no suitable revocation outputs to pay relay fee")
}
