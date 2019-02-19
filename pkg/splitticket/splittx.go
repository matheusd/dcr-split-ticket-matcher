package splitticket

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/pkg/errors"
)

// VoterLotteryPkScriptSize is the number of bytes in the output script of the
// output that stores the voter lottery commitment. The entry is:
// OP_RETURN
// OP_DATA32
// [32-byte data]
const VoterLotteryPkScriptSize = 1 + 1 + 32

// CheckSplit validates that the given split transaction respects the rules for
// the split ticket matching service
func CheckSplit(split *wire.MsgTx, utxos UtxoMap,
	secretHashes []SecretNumberHash, mainchainHash *chainhash.Hash,
	currentBlockHeight uint32, params *chaincfg.Params) error {

	maxNbInputs := MaximumSplitInputs * len(secretHashes)
	if len(split.TxIn) > maxNbInputs {
		return errors.Errorf("split ticket uses more inputs (%d) than the "+
			"maximum allowed (%d)", len(split.TxIn), maxNbInputs)
	}

	err := blockchain.CheckTransactionSanity(split, params)
	if err != nil {
		return errors.Wrap(err, "split tx failed sanity check")
	}

	if len(split.TxOut[0].PkScript) != VoterLotteryPkScriptSize {
		return errors.Errorf("size of pkscript of output 0 (%d) of split tx "+
			"doesn't have expected length (%d)", len(split.TxOut[0].PkScript),
			VoterLotteryPkScriptSize)
	}

	if split.TxOut[0].PkScript[0] != txscript.OP_RETURN {
		return errors.Errorf("output 0 of split tx is not an OP_RETURN")
	}

	if len(split.TxOut[0].PkScript) < 3 {
		return errors.Errorf("lottery commitment output too small")
	}

	// ensure the various locks don't prevent the split from being mined
	if split.Expiry != wire.NoExpiryValue {
		return errors.Errorf("expiry for the split tx is not 0")
	}
	if split.LockTime != 0 {
		return errors.Errorf("locktime for split tx is not 0")
	}
	if split.Version != wire.TxVersion {
		return errors.Errorf("split tx version (%d) different than expected "+
			"(%d)", split.Version, wire.TxVersion)
	}
	for i, in := range split.TxIn {
		if in.Sequence != wire.MaxTxInSequenceNum {
			return errors.Errorf("input %d of split tx has sequence number "+
				"(%d) different than expected (%d)", i, in.Sequence,
				wire.MaxTxInSequenceNum)
		}
	}

	for i, out := range split.TxOut {
		if out.Version != txscript.DefaultScriptVersion {
			return errors.Errorf("output %d of split tx does not use the "+
				"default script version (%d)", i, out.Version)
		}
	}

	return nil
}

// CheckSignedSplit validates that the given signed split transaction is
// valid according to split ticket matcher rules. Only safe to be called on
// split transactions that passed CheckSplit
func CheckSignedSplit(split *wire.MsgTx, utxos UtxoMap, params *chaincfg.Params) error {
	var totalAmountIn int64
	for i, in := range split.TxIn {
		utxo, hasUtxo := utxos[in.PreviousOutPoint]
		if !hasUtxo {
			return errors.Errorf("utxo for input %d of split tx not provided", i)
		}

		if in.ValueIn != wire.NullValueIn && utxo.Value != dcrutil.Amount(in.ValueIn) {
			return errors.Errorf("valueIn for input %d of split tx not equal "+
				"to corresponding utxo value", i)
		}

		// ensure the input actually signs the split transaction
		engine, err := txscript.NewEngine(utxo.PkScript, split, i,
			currentScriptFlags, utxo.Version, nil)
		if err != nil {
			return errors.Wrapf(err, "error creating engine to process input "+
				"%d of split tx", i)
		}

		err = engine.Execute()
		if err != nil {
			return errors.Wrapf(err, "error executing script of input %d of "+
				"split tx", i)
		}

		newAmountIn := totalAmountIn + int64(utxo.Value)
		if (newAmountIn < 0) || (newAmountIn > dcrutil.MaxAmount) {
			return errors.Errorf("overflow of total input amount of split tx "+
				"at index %d", i)
		}
		totalAmountIn = newAmountIn
	}

	totalAmountOut := totalOutputAmount(split)
	txFee := totalAmountIn - int64(totalAmountOut)

	serializedSize := int64(split.SerializeSize())
	minFee := (serializedSize * int64(TxFeeRate)) / 1000
	if txFee < minFee {
		return errors.Errorf("split tx fee (%s) less than minimum required "+
			"amount (%s)", dcrutil.Amount(txFee), dcrutil.Amount(minFee))
	}

	return nil
}

// CheckParticipantInSplit verifies that the given split transaction records
// the given output address for ticket participation and the specified change.
//
// This is to be used by each individual participant to verify if they are
// present in the split transaction.
func CheckParticipantInSplit(split *wire.MsgTx, splitAddress dcrutil.Address,
	commitAmount, ticketFee dcrutil.Amount, splitChange *wire.TxOut,
	params *chaincfg.Params) error {

	changeIdx := -1
	outputIdx := -1

	expectedAddr := splitAddress.EncodeAddress()
	expectedAmount := int64(commitAmount + ticketFee)

	for i, out := range split.TxOut {
		// output 0 of the split is the voter lottery commitment, so ignore it
		if i == 0 {
			continue
		}

		if splitChange != nil {
			if (bytes.Equal(splitChange.PkScript, out.PkScript)) &&
				(splitChange.Value == out.Value) &&
				(splitChange.Version == out.Version) {

				if changeIdx > -1 {
					return errors.Errorf("got the split change output twice "+
						"(%d and %d)", changeIdx, i)
				}

				changeIdx = i
			}
		}

		_, addresses, reqSigs, err := txscript.ExtractPkScriptAddrs(
			out.Version, out.PkScript, params)
		if err != nil {
			return errors.Wrapf(err, "error extracting addresses from output %d", i)
		}

		if len(addresses) != 1 {
			continue
		}

		if (addresses[0].String() == expectedAddr) &&
			(out.Value == expectedAmount) &&
			(out.Version == txscript.DefaultScriptVersion) &&
			(reqSigs == 1) {

			if outputIdx > -1 {
				return errors.Errorf("got the split output twice (%d and %d)",
					outputIdx, i)
			}

			outputIdx = i
		}
	}

	if (splitChange != nil) && (changeIdx == -1) {
		return errors.Errorf("could not find change output in split tx")
	}

	if outputIdx == -1 {
		return errors.Errorf("could not find output in split tx")
	}

	return nil
}

// CheckOnlySignedInSplit checks whether the only signed inputs (identified
// by their respective outpoints) of the ticket are the expected ones.
//
// This is used by individual participants to ensure they are only signing
// the outpoints they previously specified for a particular session.
//
// Only safe to be called on splits that have passed the CheckSplit function.
func CheckOnlySignedInSplit(split *wire.MsgTx, outpoints []wire.OutPoint) error {

	// Make an aux list of expected outpoits, so we can easily check if
	// individual inputs were expected to be signed or not.
	expected := make(map[wire.OutPoint]struct{}, len(outpoints))
	for _, outp := range outpoints {
		expected[outp] = struct{}{}
	}

	for _, in := range split.TxIn {
		_, isExpected := expected[in.PreviousOutPoint]
		isSigEmpty := len(in.SignatureScript) == 0
		if isExpected && isSigEmpty {
			return errors.Errorf("input %s was not signed by wallet",
				in.PreviousOutPoint)
		}
		if !isExpected && !isSigEmpty {
			return errors.Errorf("input %s should NOT have been signed by wallet",
				in.PreviousOutPoint)
		}

		// isExpected && !isSigEmpty means the input was successfully signed.
		// Remove from the expected map so that we verify for missing outpoints.
		if isExpected {
			delete(expected, in.PreviousOutPoint)
		}

		// !isExpected && isSigEmpty means this was not an output for the
		// wallet.
	}

	// Check if any outpoints were left in the expected map, meaning they should
	// have been signed.
	var notFoundOutps string
	for outp := range expected {
		notFoundOutps += fmt.Sprintf("%s;", outp)
	}
	if notFoundOutps != "" {
		return errors.Errorf("expected outpoints not found signed: %s",
			notFoundOutps)
	}

	return nil
}

// CheckSplitLotteryCommitment verifies whether the split transaction contains
// the correct voter commitment lottery, given the information necessary to
// derive it.
// Only safe to be called on transactions that have passed CheckSplit().
func CheckSplitLotteryCommitment(split *wire.MsgTx,
	secretHashes []SecretNumberHash, amounts []dcrutil.Amount,
	voteAddresses []dcrutil.Address, mainchainHash *chainhash.Hash) error {

	expected := CalcLotteryCommitmentHash(secretHashes, amounts, voteAddresses,
		mainchainHash)

	// pick the range [2:] because the first byte is the OP_RETURN, the second
	// is the push data op
	splitCommitment := split.TxOut[0].PkScript[2:]
	if !bytes.Equal(expected[:], splitCommitment) {
		return errors.Errorf("voter lottery commitment (%s) does not equal "+
			"the expected value (%s)", hex.EncodeToString(splitCommitment),
			hex.EncodeToString(expected[:]))
	}

	return nil
}
