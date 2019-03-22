package splitticket

// Partial Dcrdata definitions lifted from the dcrdata
// (github.com/decred/dcrdata) project.

// scriptPubKey is the result of decodescript(ScriptPubKeyHex)
type scriptPubKey struct {
	Hex string `json:"hex"`
}

// txInputID specifies a transaction input as hash:vin_index.
type txInputID struct {
	Hash  string `json:"hash"`
	Index uint32 `json:"vin_index"`
}

// vout defines a transaction output
type vout struct {
	Value               float64      `json:"value"`
	Version             uint16       `json:"version"`
	ScriptPubKeyDecoded scriptPubKey `json:"scriptPubKey"`
	Spend               *txInputID   `json:"spend,omitempty"`
}

// txShort models info about transaction TxID
type txShort struct {
	TxID string `json:"txid"`
	Vout []vout `json:"vout"`
}

// txns models the multi transaction post data structure
type txns struct {
	Transactions []string `json:"transactions"`
}
