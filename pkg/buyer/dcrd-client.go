package buyer

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"
	"io/ioutil"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrjson"
	"github.com/decred/dcrd/wire"
	"github.com/pkg/errors"

	"github.com/decred/dcrd/rpcclient"
)

type decredNetworkConfig struct {
	Host     string
	User     string
	Pass     string
	CertFile string
}

type decredNetwork struct {
	splitHash       chainhash.Hash
	ticketsHashes   []chainhash.Hash
	client          *rpcclient.Client
	publishedSplit  bool
	publishedTicket *wire.MsgTx
}

func connectToDecredNode(cfg *decredNetworkConfig) (*decredNetwork, error) {

	net := &decredNetwork{}

	// Connect to local dcrd RPC server using websockets.
	certs, err := ioutil.ReadFile(cfg.CertFile)
	if err != nil {
		return nil, err
	}
	connCfg := &rpcclient.ConnConfig{
		Host:         cfg.Host,
		Endpoint:     "ws",
		User:         cfg.User,
		Pass:         cfg.Pass,
		Certificates: certs,
	}

	ntfsHandler := &rpcclient.NotificationHandlers{
		OnTxAcceptedVerbose: net.onTxAccepted,
	}

	client, err := rpcclient.New(connCfg, ntfsHandler)
	if err != nil {
		return nil, err
	}

	err = client.NotifyNewTransactions(true)
	if err != nil {
		return nil, errors.Wrapf(err, "error subscribing to tx notifications")
	}

	net.client = client

	return net, nil
}

// monitorSession monitors the given session (split and possible tickets) for
// publishing. Assumes the split and ticket templates have been received and
// the vote/pool pkscripts of individual participants have also been received.
func (dcrd *decredNetwork) monitorSession(ctx context.Context, sess *Session) {
	dcrd.splitHash = sess.splitTx.TxHash()

	dcrd.ticketsHashes = make([]chainhash.Hash, len(sess.participants))
	for i, p := range sess.participants {
		dcrd.ticketsHashes[i] = p.ticket.TxHash()
	}
}

func (dcrd *decredNetwork) onTxAccepted(txDetails *dcrjson.TxRawResult) {
	txHash, err := chainhash.NewHashFromStr(txDetails.Txid)
	if err != nil {
		// we just ignore this wrong tx hash, as it is not relevant to us.
		return
	}

	if txHash.IsEqual(&dcrd.splitHash) {
		dcrd.publishedSplit = true
		return
	}

	for _, th := range dcrd.ticketsHashes {
		if !txHash.IsEqual(&th) {
			continue
		}

		bts, err := hex.DecodeString(txDetails.Hex)
		if err != nil {
			// maybe alert here?
			fmt.Println("err deocding hex")
			return
		}

		tx := wire.NewMsgTx()
		err = tx.FromBytes(bts)
		if err != nil {
			// maybe alert here?
			fmt.Println("err deocding tx")
			return
		}

		dcrd.publishedTicket = tx
	}
}

func (dcrd *decredNetwork) fetchSplitUtxos(split *wire.MsgTx) (splitticket.UtxoMap, error) {
	return splitticket.UtxoMapFromNetwork(dcrd.client, split)
}
