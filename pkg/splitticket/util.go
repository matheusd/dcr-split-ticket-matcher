package splitticket

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/rpcclient"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	dcrdatatypes "github.com/decred/dcrdata/api/types"
	"github.com/pkg/errors"
)

// currentScriptFlags are the flags for script execution currently enabled on
// the decred network. This might need updating in case the consensus rules
// change.
var currentScriptFlags = txscript.ScriptDiscourageUpgradableNops |
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

		if txOut == nil {
			return nil, errors.Wrapf(err, "outpoint is spent: %s", outp)
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

// UtxoMapFromDcrdata queries the dcrdata server for the outpoints of the given
// transaction and returns an utxo map for use in validation functions.
func UtxoMapFromDcrdata(dcrdataURL string, tx *wire.MsgTx) (UtxoMap, error) {

	outpoints := make([]*wire.OutPoint, len(tx.TxIn))
	for i, in := range tx.TxIn {
		outpoints[i] = &in.PreviousOutPoint
	}
	return UtxoMapOutpointsFromDcrdata(dcrdataURL, outpoints)
}

// UtxoMapOutpointsFromDcrdata queries the dcrdata server for the outpoints of
// the given transaction and returns an utxo map for use in validation
// functions.
func UtxoMapOutpointsFromDcrdata(dcrdataURL string, outpoints []*wire.OutPoint) (UtxoMap, error) {

	// Ideally, this should be a batched call, but dcrdata doesn't currently
	// have one that will return the pkscript of multiple utxos.

	client := http.Client{Timeout: time.Second * 10}
	utxos := make(UtxoMap, len(outpoints))
	respTxOut := new(dcrdatatypes.TxOut)

	for _, outp := range outpoints {
		url := fmt.Sprintf("%s/api/tx/%s/out/%d", dcrdataURL, outp.Hash.String(),
			outp.Index)

		urlResp, err := client.Get(url)
		if err != nil {
			return nil, errors.Wrapf(err, "error during GET of outpoint %s",
				outp.String())
		}

		dec := json.NewDecoder(urlResp.Body)
		err = dec.Decode(respTxOut)
		urlResp.Body.Close()
		if err != nil {
			return nil, errors.Wrap(err, "error decoding json response")
		}

		amount, err := dcrutil.NewAmount(respTxOut.Value)
		if err != nil {
			return nil, errors.Wrapf(err, "error converting outpoint %s value "+
				"(%f) to amount", outp.String(), respTxOut.Value)
		}

		pkscript, err := hex.DecodeString(respTxOut.PkScript)
		if err != nil {
			return nil, errors.Wrapf(err, "error decoding pkscript of outpoint "+
				"%s from hex", outp.String())
		}

		utxos[*outp] = UtxoEntry{
			PkScript: pkscript,
			Value:    amount,
			Version:  respTxOut.Version,
		}
	}

	return utxos, nil
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

// StakeDiffChangeDistance returns the distance (in blocks) to the closest stake
// diff change block (either in the past or the future, whichever is closest).
func StakeDiffChangeDistance(blockHeight uint32, params *chaincfg.Params) int32 {

	winSize := int32(params.WorkDiffWindowSize)

	// the remainder decreases as the block height approaches a change block, so
	// the lower baseDist is, the closer it is to the next change.
	baseDist := int32(blockHeight) % winSize
	if baseDist > winSize/2 {
		// the previous change block is closer
		return winSize - baseDist
	}

	// the next change block is closer
	return baseDist
}

// TargetTicketExpirationBlock calculates the expected expiration block for a
// ticket, given the current block height and a maximum expiry value.
//
// The calculated value is guaranteed to be < maxExpiry, but may be
// significantly less if the current block height is close to a change in stake
// difficulty.
func TargetTicketExpirationBlock(curBlockHeight, maxExpiry uint32,
	params *chaincfg.Params) uint32 {

	dist := curBlockHeight % uint32(params.WorkDiffWindowSize)
	if dist < maxExpiry {
		return curBlockHeight + dist
	}

	return curBlockHeight + maxExpiry
}
