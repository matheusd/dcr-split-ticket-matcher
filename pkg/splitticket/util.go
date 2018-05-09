package splitticket

import (
	"encoding/hex"

	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/rpcclient"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/pkg/errors"
)

// currentScriptFlags are the flags for script execution currently enabled on
// the decred network. This might need updating in case the consensus rules
// change.
var currentScriptFlags = txscript.ScriptBip16 |
	txscript.ScriptDiscourageUpgradableNops |
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

// UtxoEntry is an entry of the utxo set of the network
type UtxoEntry struct {
	PkScript []byte
	Value    dcrutil.Amount
	Version  uint16
}

// UtxoMap is an auxilary type for the split transaction checks.
type UtxoMap map[wire.OutPoint]UtxoEntry

// UtxoMapFromNetwork queries a daemon connected via rpc for the outpoints of
// the given transaction and returns an utxo map for use in validation
// functions.
func UtxoMapFromNetwork(client *rpcclient.Client, tx *wire.MsgTx) (UtxoMap, error) {

	outpoints := make([]*wire.OutPoint, len(tx.TxIn))
	for i, in := range tx.TxIn {
		outpoints[i] = &in.PreviousOutPoint
	}
	return UtxoMapOutpointsFromNetwork(client, outpoints)
}

// UtxoMapOutpointsFromNetwork queries a daemon connected via rpc for the outpoints of
// the given transaction and returns an utxo map for use in validation
// functions.
func UtxoMapOutpointsFromNetwork(client *rpcclient.Client, outpoints []*wire.OutPoint) (UtxoMap, error) {
	res := make(UtxoMap, len(outpoints))

	for _, outp := range outpoints {

		txOut, err := client.GetTxOut(&outp.Hash, outp.Index, false)
		if err != nil {
			return nil, errors.Wrapf(err, "error obtaining utxo %s", outp)
		}

		pkScript, err := hex.DecodeString(txOut.ScriptPubKey.Hex)
		if err != nil {
			return nil, errors.Wrapf(err, "error decoding pkscript of "+
				"outpoint %s", outp)
		}

		amount, err := dcrutil.NewAmount(txOut.Value)
		if err != nil {
			return nil, errors.Wrapf(err, "error decoding utxo amount of %s",
				outp)
		}

		res[*outp] = UtxoEntry{
			PkScript: pkScript,
			Value:    amount,
			Version:  uint16(txOut.Version),
		}
	}

	return res, nil
}

// FindTxFee finds the total transaction fee paid on the the given transaction
// This function does **not** check for overflows.
func FindTxFee(tx *wire.MsgTx, utxos UtxoMap) (dcrutil.Amount, error) {

	totalOut := dcrutil.Amount(0)
	for _, out := range tx.TxOut {
		totalOut += dcrutil.Amount(out.Value)
	}

	totalIn := dcrutil.Amount(0)
	for i, in := range tx.TxIn {
		utxo, has := utxos[in.PreviousOutPoint]
		if !has {
			return 0, errors.Errorf("outpoint of input %d of split tx (%s)"+
				"not found", i, in.PreviousOutPoint)
		}
		totalIn += utxo.Value
	}

	return totalIn - totalOut, nil
}
