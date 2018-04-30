package splitticket

import (
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

		txRes, err := client.GetRawTransaction(&outp.Hash)
		if err != nil {
			return nil, errors.Wrapf(err, "error obtaining tx %s", outp.Hash)
		}

		tx := txRes.MsgTx()

		if outp.Index >= uint32(len(tx.TxOut)) {
			return nil, errors.Errorf("tx does not have output of index %d",
				outp.Index)
		}

		outRes := tx.TxOut[outp.Index]

		res[*outp] = UtxoEntry{
			PkScript: outRes.PkScript,
			Value:    dcrutil.Amount(outRes.Value),
			Version:  outRes.Version,
		}
	}

	return res, nil
}
