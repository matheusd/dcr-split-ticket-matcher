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

// CheckTicket validates that the given ticket respects the rules for the
// split ticket matching service. Split must have passed the CheckSplit()
// function.
// This function can be called on buyers, to ensure that no other participant
// or the matcher service are trying to trick the buyer into a malicious
// split ticket session, wasting time or (more importantly) funds.
func CheckTicket(split, ticket *wire.MsgTx, ticketPrice, partPoolFee,
	partTicketFee dcrutil.Amount, partsAmounts []dcrutil.Amount,
	params *chaincfg.Params) error {

	var err error

	// ensure the ticket tx looks like a valid decred tx
	err = blockchain.CheckTransactionSanity(ticket, params)
	if err != nil {
		return errors.Wrap(err, "ticket failed sanity check")
	}

	// ensure this actually validates as an sstx
	err = stake.CheckSStx(ticket)
	if err != nil {
		return errors.Wrap(err, "ticket tx is not an sstx")
	}

	// ensure the ticket has the appropriate number of inputs, given how many
	// participants are in the split ticket
	if len(ticket.TxIn) != len(partsAmounts)+1 {
		return errors.Errorf("ticket has different number of inputs (%d) "+
			"than expected (%d)", len(ticket.TxIn), len(partsAmounts))
	}

	// ensure the ticket has the appropriate number of outputs, given how
	// many participants are in the split ticket
	// number of participants + voting addr + pool fee + pool fee change
	expectedNbOuts := len(partsAmounts)*2 + 1 + 2
	if len(ticket.TxOut) != expectedNbOuts {
		return errors.Errorf("ticket has different number of outputs (%d) "+
			" than expected (%d)", len(ticket.TxOut), expectedNbOuts)
	}

	// ensure the output amount is the specified ticket price
	if dcrutil.Amount(ticket.TxOut[0].Value) != ticketPrice {
		return errors.Errorf("ticket price has different value (%s) than "+
			"expected (%s)", dcrutil.Amount(ticket.TxOut[0].Value), ticketPrice)
	}

	splitHash := split.TxHash()

	var totalAmountIn int64
	var amountsIn []int64
	for i, in := range ticket.TxIn {
		// ensure all ticket inputs come from the split transaction
		if in.PreviousOutPoint.Index >= uint32(len(split.TxOut)) {
			return errors.Errorf("input %d of ticket references inexistent "+
				"output %d of split tx", i, in.PreviousOutPoint.Index)
		}

		if !splitHash.IsEqual(&in.PreviousOutPoint.Hash) {
			return errors.Errorf("input %d of ticket does not reference "+
				"tx hash", i)
		}

		out := split.TxOut[in.PreviousOutPoint.Index]

		// ensure the split output/ticket input value is actually the same as
		// the expected participant amount.
		if i > 0 {
			expected := partsAmounts[i-1] + partTicketFee
			amount := dcrutil.Amount(out.Value)
			if amount != expected {
				return errors.Errorf("amount in ticket input %d (%s) different "+
					"than expected (%s)", i, amount, expected)
			}
		}

		// ensure the input actually signs the ticket transaction
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

		// ensure the input amounts are not overflowing the accumulator
		// (CheckTransactionSanity does this for outputs)
		newAmountIn := totalAmountIn + out.Value
		if (newAmountIn <= 0) || (newAmountIn >= dcrutil.MaxAmount) {
			return errors.Errorf("input amount of ticket overflows maximum " +
				"tx input amount")
		}
		totalAmountIn = newAmountIn
		amountsIn = append(amountsIn, out.Value)
	}

	// ensure the commitment amounts are congruent with the inputs.
	// Current decred consensus rules enforce commitments proportional to inputs.
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

	// ensure the spending limit allowance is the default one.
	for i, limit := range spendLimits {
		if (limit[0] != 0) || (limit[1] != 24) {
			return errors.Errorf("limit of output %d is not the standard value", i)
		}
	}

	// ensure no change amounts are being sent. We may want to change this in
	// the future if the split transaction isn't needed anymore and we actually
	// start accepting doing the coinjoin directly on the ticket transaction.
	for i, amount := range chgAmt {
		if amount != 0 {
			return errors.Errorf("ticket change %d has amount > 0 (%s)", i,
				dcrutil.Amount(amount))
		}
	}

	// we expect all participants to pay the same amount as pool fee
	expectedPoolFee := partPoolFee * dcrutil.Amount(len(partsAmounts))

	// ensure the pool fee commitment actually is the total pool fee
	if dcrutil.Amount(outAmt[0]) != expectedPoolFee {
		return errors.Errorf("amount in commitment for pool fee (%s) "+
			"different than expected (%s)", dcrutil.Amount(outAmt[0]),
			expectedPoolFee)
	}

	// ensure the participation amounts in the ticket actually follow the
	// provided distribution. This is specially important because we'll decide
	// the voter based on the amounts in `partsAmounts`, so it's **very**
	// important to ensure this isn't changing on the actual ticket.
	for i := 1; i < len(outAmt); i++ {
		expected := partsAmounts[i-1] + partTicketFee
		amount := dcrutil.Amount(outAmt[i])
		if amount != expected {
			return errors.Errorf("amount in commitment %d (%s) different "+
				"than expected (%s)", i, amount, expected)
		}
	}

	// ensure that the pool fee is congruent with what is observed in the real
	// network (5% max on mainnet, 7.5% max on testnet/simnet). We can check
	// using the poolFee/ticketPrice on the arguments because we're also
	// validating elsewhere that these are correct in the actual ticket tx.
	poolFeeRate := float64(expectedPoolFee) / float64(ticketPrice)
	if params.Name == "mainnet" && poolFeeRate > 0.05 {
		return errors.Errorf("pool fee rate (%f) higher than expected for mainnet")
	} else if poolFeeRate > 0.075 {
		return errors.Errorf("pool fee rate (%f) higher than expected for testnet")
	}

	// ensure that the ticket fee being used will actually allow the ticket to be
	// mined (fee rate much lower than 0.001 DCR/KB might block the ticket)
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

	// ensure that the ticket fee is not higher than some arbitrary threshold
	// to prevent fee drain. We might wanna lower this later on.
	if txFee > 2*minFee {
		return errors.Errorf("ticket fee (%s) higher than 2 times minimum required amount (%s)",
			dcrutil.Amount(txFee), dcrutil.Amount(2*minFee))
	}

	return nil
}

// CheckTicketScriptMatchAddresses checks whether the voteaddress is actually
// present in the vote pk script and if the pool address is present in the
// poolPkScript
func CheckTicketScriptMatchAddresses(voteAddress, poolAddress dcrutil.Address,
	votePkScript, poolPkScript []byte, poolFee dcrutil.Amount,
	params *chaincfg.Params) error {

	// validating the vote pk script
	voteClass, voteAddresses, voteReqSigs, err := txscript.ExtractPkScriptAddrs(
		txscript.DefaultScriptVersion, votePkScript, params)
	if err != nil {
		errors.Wrapf(err, "error decoding vote pkscript")
	}

	if voteClass != txscript.StakeSubmissionTy {
		return errors.Errorf("vote pkscript (%s) is not a StakeSubmissionTy",
			voteClass)
	}

	if len(voteAddresses) != 1 {
		return errors.Errorf("decoded different number of vote addresses "+
			"(%d) than expected", len(voteAddresses))
	}

	if voteReqSigs != 1 {
		return errors.Errorf("more than 1 signature required on vote pkscript")
	}

	if voteAddress.String() != voteAddresses[0].String() {
		return errors.Errorf("decoded vote address on script (%s) does not "+
			"match the expected vote address (%s)", voteAddresses[0],
			voteAddress)
	}

	// validating the pool pk script
	poolClass := txscript.GetScriptClass(txscript.DefaultScriptVersion,
		poolPkScript)

	if poolClass != txscript.NullDataTy {
		return errors.Errorf("pool pkscript (%s) is not a NullDataTy",
			poolClass)
	}

	decodedPoolAddr, err := stake.AddrFromSStxPkScrCommitment(poolPkScript, params)
	if err != nil {
		return errors.Wrapf(err, "error decoding pool commitment address")
	}

	if poolAddress.String() != decodedPoolAddr.String() {
		return errors.Errorf("decoded pool address on script (%s) does not "+
			"match the expected pool address (%s)", decodedPoolAddr,
			poolAddress)
	}

	decodedPoolAmount, err := stake.AmountFromSStxPkScrCommitment(poolPkScript)
	if decodedPoolAmount != poolFee {
		return errors.Errorf("decoded pool fee (%s) is not equal to expected "+
			"pool fee (%s)", decodedPoolAmount, poolFee)
	}

	return nil

}
