package splitticket

import (
	"bytes"
	"encoding/hex"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/pkg/errors"
)

const (
	// MaxPoolFeeRateTestnet is the maximum observed pool fee rate in
	// testnet/simnet (%)
	MaxPoolFeeRateTestnet = 7.5

	// MaxPoolFeeRateMainnet is the maximum observed pool fee rate in mainnet (%)
	MaxPoolFeeRateMainnet = 5.0

	// MaximumTicketExpiry is the maximum expiry (in blocks) expected in a ticket
	// transaction
	MaximumTicketExpiry = 16
)

// CheckTicket validates that the given ticket respects the rules for the
// split ticket matching service. Split must have passed the CheckSplit()
// function.
// This function can be called on buyers, to ensure that no other participant
// or the matcher service are trying to trick the buyer into a malicious
// split ticket session, wasting time or (more importantly) funds.
func CheckTicket(split, ticket *wire.MsgTx, ticketPrice, partPoolFee,
	partTicketFee dcrutil.Amount, partsAmounts []dcrutil.Amount,
	currentBlockHeight uint32, params *chaincfg.Params) error {

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

		if in.PreviousOutPoint.Tree != wire.TxTreeRegular {
			return errors.Errorf("input %d of ticket does not reference the "+
				"regular tx tree", i)
		}

		if !splitHash.IsEqual(&in.PreviousOutPoint.Hash) {
			return errors.Errorf("input %d of ticket does not reference "+
				"tx hash of split tx", i)
		}

		if in.Sequence != wire.MaxTxInSequenceNum {
			return errors.Errorf("input %d of ticket has sequence number "+
				"(%d) different than expected (%d)", i, in.Sequence,
				wire.MaxTxInSequenceNum)
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
	poolFeeRate := (float64(expectedPoolFee) / float64(ticketPrice)) * 100
	if params.Name == "mainnet" && poolFeeRate > MaxPoolFeeRateMainnet {
		return errors.Errorf("pool fee rate (%f) higher than expected for "+
			"mainnet", poolFeeRate)
	} else if poolFeeRate > MaxPoolFeeRateTestnet {
		return errors.Errorf("pool fee rate (%f) higher than expected for "+
			"testnet", poolFeeRate)
	}

	// ensure the various locks don't prevent the ticket from being mined
	if ticket.Expiry == 0 {
		return errors.Errorf("expiry for the ticket is 0")
	}
	if ticket.LockTime != 0 {
		return errors.Errorf("locktime for ticket is not 0")
	}
	if ticket.Version != wire.TxVersion {
		return errors.Errorf("ticket tx version (%d) different than expected "+
			"(%d)", ticket.Version, wire.TxVersion)
	}

	// ensure the expiry doesn't leave the ticket eternally on mempool
	expiryDist := ticket.Expiry - currentBlockHeight
	if expiryDist > MaximumTicketExpiry {
		return errors.Errorf("expiry (%d) is greater than maximum allowed (%d)",
			expiryDist, MaximumTicketExpiry)
	}

	for i, out := range ticket.TxOut {
		if out.Version != txscript.DefaultScriptVersion {
			return errors.Errorf("output %d of ticket does not use the "+
				"default script version (%d)", i, out.Version)
		}
	}

	return nil
}

// CheckSignedTicket validates whether the given signed ticket can be spent
// on the network. Only safe to be called on tickets that passed CheckTicket().
func CheckSignedTicket(split, ticket *wire.MsgTx, params *chaincfg.Params) error {
	for i, in := range ticket.TxIn {
		out := split.TxOut[in.PreviousOutPoint.Index]

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
	}

	var totalAmountIn int64
	for _, in := range ticket.TxIn {
		out := split.TxOut[in.PreviousOutPoint.Index]
		totalAmountIn += out.Value
	}

	// ensure that the ticket fee being used will actually allow the ticket to be
	// mined (fee rate much lower than 0.001 DCR/KB might block the ticket)
	totalAmountOut := ticket.TxOut[0].Value
	if totalAmountOut >= totalAmountIn {
		return errors.Errorf("total output amount in ticket (%s) >= "+
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
		return errors.Wrapf(err, "error decoding vote pkscript")
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
	if err != nil {
		return errors.Wrapf(err, "error extracing amount from commitment script")
	}
	if decodedPoolAmount != poolFee {
		return errors.Errorf("decoded pool fee (%s) is not equal to expected "+
			"pool fee (%s)", decodedPoolAmount, poolFee)
	}

	return nil

}

// CheckParticipantInTicket checks whether the participant at `index` on the
// ticket is present with the given parameters.
//
// This function checks whether the input and commitment output of the
// ith participant is being committed to the correct address and amount.
//
// This is only safe to be called on tickets that have passed the CheckTicket
// function.
func CheckParticipantInTicket(split, ticket *wire.MsgTx, amount,
	fee dcrutil.Amount, commitmentAddr, splitAddr dcrutil.Address,
	splitChange *wire.TxOut, index uint32,
	splitInputs []wire.OutPoint, params *chaincfg.Params) error {

	// the first input is the pool fee, so skip that to get the real input idx
	idxInput := index + 1

	// output 0 is vote, output 1 is the pool commitment, output 2 is pool
	// change, so the first commitment is at index 3
	idxOutput := 3 + index*2

	out := ticket.TxOut[idxOutput]

	decodedAddr, err := stake.AddrFromSStxPkScrCommitment(out.PkScript, params)
	if err != nil {
		return errors.Wrapf(err, "error decoding commitment address")
	}

	if decodedAddr.String() != commitmentAddr.String() {
		return errors.Errorf("commitment address (%s) at index %d not equal "+
			"to expected address (%s)", decodedAddr, idxOutput, commitmentAddr)
	}

	contrib := amount + fee

	decodedAmount, err := stake.AmountFromSStxPkScrCommitment(out.PkScript)
	if err != nil {
		return errors.Wrapf(err, "error extracting amount from commitment script")
	}
	if decodedAmount != contrib {
		return errors.Errorf("decoded commitment amount (%s) is not equal to "+
			"expected amount (%s)", decodedAmount, contrib)
	}

	in := ticket.TxIn[idxInput]
	splitOut := split.TxOut[in.PreviousOutPoint.Index]

	if dcrutil.Amount(splitOut.Value) != contrib {
		return errors.Errorf("input amount for ticket (%s) is not equal to "+
			"expected amount (%s)", dcrutil.Amount(splitOut.Value), contrib)
	}

	class, addresses, reqSigs, err := txscript.ExtractPkScriptAddrs(
		splitOut.Version, splitOut.PkScript, params)
	if err != nil {
		return errors.Wrapf(err, "error decoding pkscript of split output")
	}

	if class != txscript.PubKeyHashTy {
		return errors.Errorf("split output script is not PubKeyHashTy")
	}

	if reqSigs != 1 {
		return errors.Errorf("split output script requires a different "+
			"number of signatures (%d) than expected", reqSigs)
	}

	if len(addresses) != 1 {
		return errors.Errorf("split output script has a different number of "+
			"addresses (%d) than expected", len(addresses))
	}

	if addresses[0].String() != splitAddr.String() {
		return errors.Errorf("address in output (%s) does not match the"+
			"expected address (%s)", addresses[0], splitAddr)
	}

	splitOutPoints := make(map[wire.OutPoint]bool, len(split.TxIn))
	for _, in := range split.TxIn {
		splitOutPoints[in.PreviousOutPoint] = true
	}
	for _, expectedOutp := range splitInputs {
		if _, has := splitOutPoints[expectedOutp]; !has {
			return errors.Errorf("could not find expected split outpoint "+
				"%s:%d in split inputs", hex.EncodeToString(expectedOutp.Hash[:]),
				expectedOutp.Index)
		}
	}

	found := false
	for _, out := range split.TxOut {
		if (out.Value != splitChange.Value) ||
			(out.Version != splitChange.Version) ||
			(!bytes.Equal(out.PkScript, splitChange.PkScript)) {
			continue
		}

		found = true
		break
	}

	if !found {
		return errors.Errorf("could not find change output in split tx")
	}

	return nil
}

// FindTicketTxFee finds the ticket transaction fee, assuming the split
// ticket is correct. Only safe to be called on split and ticket transactions
// that have passed their respective check functions.
func FindTicketTxFee(splitTx, ticket *wire.MsgTx) (dcrutil.Amount, error) {
	splitUtxos := make(UtxoMap, len(splitTx.TxOut))
	splitHash := splitTx.TxHash()
	for i, out := range splitTx.TxOut {
		outp := wire.OutPoint{
			Hash:  splitHash,
			Index: uint32(i),
			Tree:  wire.TxTreeRegular,
		}
		splitUtxos[outp] = UtxoEntry{
			PkScript: out.PkScript,
			Value:    dcrutil.Amount(out.Value),
			Version:  out.Version,
		}
	}

	return FindTxFee(ticket, splitUtxos)
}
