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
func CheckTicket(split, ticket *wire.MsgTx, ticketPrice, poolFee, partTicketFee dcrutil.Amount,
	partsAmounts []dcrutil.Amount, params *chaincfg.Params) error {
	var err error

	err = blockchain.CheckTransactionSanity(ticket, params)
	if err != nil {
		return errors.Wrap(err, "ticket failed sanity check")
	}

	err = stake.CheckSStx(ticket)
	if err != nil {
		return errors.Wrap(err, "ticket tx is not an sstx")
	}

	if len(ticket.TxIn) != len(partsAmounts)+1 {
		return errors.Errorf("ticket has different number of inputs (%d) "+
			"than expected (%d)", len(ticket.TxIn), len(partsAmounts))
	}

	// number of participants + voting addr + pool fee + pool fee change
	expectedNbOuts := len(partsAmounts)*2 + 1 + 2
	if len(ticket.TxOut) != expectedNbOuts {
		return errors.Errorf("ticket has different number of outputs (%d) "+
			" than expected (%d)", len(ticket.TxOut), expectedNbOuts)
	}

	if dcrutil.Amount(ticket.TxOut[0].Value) != ticketPrice {
		return errors.Errorf("ticket price has different value (%s) than "+
			"expected (%s)", dcrutil.Amount(ticket.TxOut[0].Value), ticketPrice)
	}

	splitHash := split.TxHash()

	var totalAmountIn int64
	var amountsIn []int64
	for i, in := range ticket.TxIn {
		if in.PreviousOutPoint.Index >= uint32(len(split.TxOut)) {
			return errors.Errorf("input %d of ticket references inexistent "+
				"output %d of split tx", i, in.PreviousOutPoint.Index)
		}

		if !splitHash.IsEqual(&in.PreviousOutPoint.Hash) {
			return errors.Errorf("input %d of ticket does not reference "+
				"tx hash", i)
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
			return errors.Wrapf(err, "error executing script of input %d of "+
				"ticket", i)
		}

		newAmountIn := totalAmountIn + out.Value
		if (newAmountIn <= 0) || (newAmountIn >= dcrutil.MaxAmount) {
			return errors.Errorf("input amount of ticket overflows maximum " +
				"tx input amount")
		}
		totalAmountIn = newAmountIn
		amountsIn = append(amountsIn, out.Value)
	}

	_, _, outAmt, chgAmt, _, spendLimits := stake.TxSStxStakeOutputInfo(ticket)
	_, outAmtCalc, err := stake.SStxNullOutputAmounts(amountsIn, chgAmt,
		int64(ticketPrice))
	if err != nil {
		return errors.Wrap(err, "error extracting ticket commitment values")
	}

	err = stake.VerifySStxAmounts(outAmt, outAmtCalc)
	if err != nil {
		return errors.Wrap(err, "ticket commitment amounts different than "+
			"calculated")
	}

	for i, limit := range spendLimits {
		if (limit[0] != 0) || (limit[1] != 24) {
			return errors.Errorf("limit of output %d is not the standard value", i)
		}
	}

	for i, amount := range chgAmt {
		if amount != 0 {
			return errors.Errorf("ticket change %d has amount > 0 (%s)", i,
				dcrutil.Amount(amount))
		}
	}

	if dcrutil.Amount(outAmt[0]) != poolFee {
		return errors.Errorf("amount in commitment for pool fee (%s) "+
			"different than expected (%s)", dcrutil.Amount(outAmt[0]), poolFee)
	}

	for i := 1; i < len(outAmt); i++ {
		expected := partsAmounts[i-1] + partTicketFee
		amount := dcrutil.Amount(outAmt[i])
		if amount != expected {
			return errors.Errorf("amount in commitment %d (%s) different "+
				"than expected (%s)", i, amount, expected)
		}
	}

	poolFeeRate := float64(poolFee) / float64(ticketPrice)
	if params.Name == "mainnet" && poolFeeRate > 0.05 {
		return errors.Errorf("pool fee rate (%f) higher than expected for mainnet")
	} else if poolFeeRate > 0.075 {
		return errors.Errorf("pool fee rate (%f) higher than expected for testnet")
	}

	// check if the tx fee for the ticket is >= default fee
	totalAmountOut := ticket.TxOut[0].Value
	if totalAmountOut >= totalAmountIn {
		return errors.Wrapf(err, "total output amount in ticket (%s) >= "+
			"total input amount (%s)", dcrutil.Amount(totalAmountOut),
			dcrutil.Amount(totalAmountIn))
	}
	txFee := totalAmountIn - totalAmountOut
	serializedSize := int64(ticket.SerializeSize())
	minFee := (serializedSize * int64(minRelayFeeRate)) / 1000
	if txFee < minFee {
		return errors.Errorf("ticket fee (%s) less than minimum required amount (%s)",
			dcrutil.Amount(txFee), dcrutil.Amount(minFee))
	}

	expectedFee := partTicketFee * dcrutil.Amount(len(partsAmounts))
	if dcrutil.Amount(txFee) != expectedFee {
		return errors.Errorf("ticket fee (%s) different than expected (%s)",
			dcrutil.Amount(txFee), expectedFee)
	}

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
		return errors.Errorf("revocation fee (%s) less than minimum required "+
			"amount (%s)", fee, minFee)
	}

	// TODO: check if fee allowance is correct

	return nil
}
