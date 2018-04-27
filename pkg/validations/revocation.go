package validations

import (
	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/pkg/errors"
)

// CheckRevocation checks whether the revocation for the given ticket respects
// the rules for split ticket buying. The ticket must have passed the
// CheckTicket function.
func CheckRevocation(ticket, revocation *wire.MsgTx, params *chaincfg.Params) error {
	var err error

	// ensure this looks like a decred transaction
	err = blockchain.CheckTransactionSanity(revocation, params)
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
	minFee := dcrutil.Amount((serializedSize * int64(minRelayFeeRate)) / 1000)
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
