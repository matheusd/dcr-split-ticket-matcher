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

// UtxoMapFromNetwork queries a daemon connected via rpc for the outpoints of
// the given transaction and returns an utxo map for use in validation
// functions.
func UtxoMapFromNetwork(client *rpcclient.Client, tx *wire.MsgTx) (UtxoMap, error) {
	res := make(UtxoMap, len(tx.TxIn))

	for i, in := range tx.TxIn {
		outRes, err := client.GetTxOut(&in.PreviousOutPoint.Hash, in.PreviousOutPoint.Index,
			false)
		if err != nil {
			return nil, errors.Wrapf(err, "error obtaining oupoint %d of tx", i)
		}

		pkScript, err := hex.DecodeString(outRes.ScriptPubKey.Hex)
		if err != nil {
			return nil, errors.Wrapf(err, "error decoding pkscript of "+
				"outpoint %d of tx", i)
		}

		value, err := dcrutil.NewAmount(outRes.Value)
		if err != nil {
			return nil, errors.Wrapf(err, "error decoding value for outpoint "+
				"%d of tx", i)
		}

		res[in.PreviousOutPoint] = UtxoEntry{
			PkScript: pkScript,
			Value:    value,
			Version:  uint16(outRes.Version),
		}
	}

	return res, nil
}

// UtxoEntry is an entry of the utxo set of the network
type UtxoEntry struct {
	PkScript []byte
	Value    dcrutil.Amount
	Version  uint16
}

// UtxoMap is an auxilary type for the split transaction checks.
type UtxoMap map[wire.OutPoint]UtxoEntry
