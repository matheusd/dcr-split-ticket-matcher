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

// currentScriptFlags are the flags for script execution currently enabled on
// the decred network. This might need updating in case the consensus rules
// change.
var currentScriptFlags = txscript.ScriptBip16 |
	txscript.ScriptVerifyDERSignatures |
	txscript.ScriptVerifyStrictEncoding |
	txscript.ScriptVerifyMinimalData |
	txscript.ScriptVerifyCleanStack |
	txscript.ScriptVerifyCheckLockTimeVerify |
	txscript.ScriptVerifyCheckSequenceVerify |
	txscript.ScriptVerifySHA256

// minRelayFeeRate is the minimum tx fee rate for inclusion on the network.
// Measured as Atoms/KB. 1e5 = 0.001 DCR
const minRelayFeeRate dcrutil.Amount = 1e5

// totalOutputAmount calculates the total amount of outputs for a transaction.
// Only safe to be called on transactions which passed
// blockchain.CheckTransactionSanity()
func totalOutputAmount(tx *wire.MsgTx) dcrutil.Amount {
	var total int64
	for _, out := range tx.TxOut {
		total += out.Value
	}
	return dcrutil.Amount(total)
}

// CheckSplit validates that the given split transaction respects the rules for
// the split ticket matching service
func CheckSplit(split *wire.MsgTx, params *chaincfg.Params) error {
	var err error

	err = blockchain.CheckTransactionSanity(split, params)
	if err != nil {
		return errors.Wrap(err, "split tx failed sanity check")
	}

	// TODO: check if relay fee is correct
	// TODO: check if outputs follow participation amounts
	// TODO: check if output[0] is voter lottery commitment
	// TODO: check if the commitment amounts sum up to ticketPrice - totalPoolFee
	// TODO: check if poolFee is valid (5%)
	// TODO: check if poolFee is equal among participants
	// TODO: check if split tx is correctly funded
	// TODO: check if my vote/pool script are in the list
	// TODO: check if limit fees (fee allowance) is correct
	// TODO: check if split[0] has the OP_RETURN with the correct hash for voting fraud detection

	return nil
}

// CheckTicket validates that the given ticket respects the rules for the
// split ticket matching service. Split must have passed the CheckSplit()
// function.
func CheckTicket(split, ticket *wire.MsgTx, params *chaincfg.Params) error {
	var err error

	err = blockchain.CheckTransactionSanity(ticket, params)
	if err != nil {
		return errors.Wrap(err, "ticket failed sanity check")
	}

	err = stake.CheckSStx(ticket)
	if err != nil {
		return errors.Wrap(err, "ticket tx is not an sstx")
	}

	splitHash := split.TxHash()

	var amountIn int64
	for i, in := range ticket.TxIn {
		if in.PreviousOutPoint.Index >= uint32(len(split.TxOut)) {
			return errors.Errorf("input %d of ticket references inexistent "+
				"output %d of split tx", i, in.PreviousOutPoint.Index)
		}

		if !splitHash.IsEqual(&in.PreviousOutPoint.Hash) {
			return errors.Errorf("input %d of ticket does not reference split tx hash", i)
		}

		out := split.TxOut[in.PreviousOutPoint.Index]
		engine, err := txscript.NewEngine(out.PkScript, ticket, i,
			currentScriptFlags, out.Version, nil)
		if err != nil {
			return errors.Wrapf(err, "error creating engine to process input "+
				"%d of ticket", i)
		}

		err = engine.Execute()
		if err != nil {
			return errors.Wrapf(err, "error executing script of input %d of ticket", i)
		}

		newAmountIn := amountIn + out.Value
		if (newAmountIn <= 0) || (newAmountIn >= dcrutil.MaxAmount) {
			return errors.Errorf("input amount of ticket overflows maximum tx input amount")
		}
		amountIn = newAmountIn
	}

	// check if the tx fee for the ticket is >= default fee
	amountOut := ticket.TxOut[0].Value
	if amountOut >= amountIn {
		return errors.Wrapf(err, "total output amount in ticket (%s) >= "+
			"total input amount (%s)", dcrutil.Amount(amountOut), dcrutil.Amount(amountIn))
	}
	fee := amountIn - amountOut
	serializedSize := int64(ticket.SerializeSize())
	minFee := (serializedSize * int64(minRelayFeeRate)) / 1000
	if fee < minFee {
		return errors.Errorf("ticket fee (%s) less than minimum required amount (%s)",
			dcrutil.Amount(fee), dcrutil.Amount(minFee))
	}

	// TODO: check limits for commitments
	// TODO: check if commitment distribution is ok
	// TODO: check if commitment distribution follows participation amounts

	return nil
}

// CheckRevocation checks whether the revocation for the given ticket respects
// the rules for split ticket buying. The ticket must have passed the
// CheckTicket function.
func CheckRevocation(ticket, revocation *wire.MsgTx, params *chaincfg.Params) error {
	var err error

	err = blockchain.CheckTransactionSanity(revocation, params)
	if err != nil {
		return errors.Wrap(err, "revocation failed sanity check")
	}

	err = stake.CheckSSRtx(revocation)
	if err != nil {
		return errors.Wrap(err, "revocation tx is not an rrtx")
	}

	// check whether the revocation scriptSig successfully signs the ticket output
	engine, err := txscript.NewEngine(ticket.TxOut[0].PkScript, revocation, 0,
		currentScriptFlags, ticket.TxOut[0].Version, nil)
	if err != nil {
		return errors.Wrap(err, "error creating script engine")
	}
	err = engine.Execute()
	if err != nil {
		return errors.Wrap(err, "error verifying revocation scriptSig")
	}

	// check if the tx fee for the revocation is >= default fee
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
		return errors.Errorf("revocation fee (%s) less than minimum required amount (%s)",
			fee, minFee)
	}

	// TODO: check if fee allowance is correct

	return nil
}
