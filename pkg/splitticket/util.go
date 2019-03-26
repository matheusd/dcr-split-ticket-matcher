package splitticket

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/rpcclient"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
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
	PkScript      []byte
	Value         dcrutil.Amount
	Version       uint16
	Confirmations int64
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
			return nil, errors.Errorf("outpoint is spent/unknown: %s", outp)
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
			PkScript:      pkScript,
			Value:         amount,
			Version:       uint16(txOut.Version),
			Confirmations: txOut.Confirmations,
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

	// Prepare request for a batch of transactions. We should only request
	// transactions once, so keep track of which ones were already requested.
	var reqTxns txns
	txMap := make(map[chainhash.Hash]struct{}, len(outpoints))
	for _, outp := range outpoints {
		if _, has := txMap[outp.Hash]; !has {
			reqTxns.Transactions = append(reqTxns.Transactions, outp.Hash.String())
			txMap[outp.Hash] = struct{}{}
		}
	}
	var reqBuff bytes.Buffer
	enc := json.NewEncoder(&reqBuff)
	err := enc.Encode(&reqTxns)
	if err != nil {
		return nil, errors.Wrapf(err, "error encoding txs for dcrdata request")
	}

	// Perform the request and decode the response.
	//
	// TODO: verify spend status once ?spends=true works.
	client := http.Client{Timeout: time.Second * 10}
	var respTxns []txShort
	url := fmt.Sprintf("%s/api/txs", dcrdataURL)
	urlResp, err := client.Post(url, "application/json", &reqBuff)
	if err != nil {
		return nil, errors.Wrapf(err, "error during POST of dcrdata txns")
	}
	if urlResp.StatusCode != 200 {
		return nil, fmt.Errorf("dcrdata returned http error %d (%s)",
			urlResp.StatusCode, urlResp.Status)
	}
	dec := json.NewDecoder(urlResp.Body)
	err = dec.Decode(&respTxns)
	if err != nil {
		return nil, errors.Wrapf(err, "error decoding dcrdata txns response")
	}

	// Create aux map from hash into tx info so that we can alter find it.
	respTxnsMap := make(map[chainhash.Hash]*txShort, len(respTxns))
	for i := 0; i < len(respTxns); i++ {
		tx := &respTxns[i]
		txid := new(chainhash.Hash)
		err = chainhash.Decode(txid, tx.TxID)
		if err != nil {
			return nil, errors.Wrapf(err, "error decoding txhash (%s) from "+
				"dcrdata tx %d", tx.TxID, i)
		}

		respTxnsMap[*txid] = tx
	}

	// Build the output utxo map. Iterate over the requested outpoints, find
	// the response tx info and fill the data.
	utxos := make(UtxoMap, len(outpoints))
	for _, outp := range outpoints {

		tx, has := respTxnsMap[outp.Hash]
		if !has {
			return nil, fmt.Errorf("dcrdata did not return info for tx %s",
				outp.Hash.String())
		}

		if len(tx.Vout) < int(outp.Index) {
			return nil, fmt.Errorf("tx %s returned by dcrdata does not have "+
				"output %d", outp.Hash.String(), outp.Index)
		}

		txout := tx.Vout[outp.Index]

		amount, err := dcrutil.NewAmount(txout.Value)
		if err != nil {
			return nil, errors.Wrapf(err, "error converting outpoint %s value "+
				"(%f) to amount", outp.String(), txout.Value)
		}

		pkscript, err := hex.DecodeString(txout.ScriptPubKeyDecoded.Hex)
		if err != nil {
			return nil, errors.Wrapf(err, "error decoding pkscript of outpoint "+
				"%s from hex", outp.String())
		}

		if len(pkscript) == 0 {
			return nil, fmt.Errorf("dcrdata returned outpoint %s with empty "+
				"pkscript", outp.String())
		}

		utxos[*outp] = UtxoEntry{
			PkScript: pkscript,
			Value:    amount,
			Version:  txout.Version,
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
