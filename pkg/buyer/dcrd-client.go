package buyer

import (
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"
	"io/ioutil"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"

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

	ntfsHandler := &rpcclient.NotificationHandlers{}

	client, err := rpcclient.New(connCfg, ntfsHandler)
	if err != nil {
		return nil, err
	}

	net.client = client

	return net, nil
}

func (dcrd *decredNetwork) fetchSplitUtxos(split *wire.MsgTx) (splitticket.UtxoMap, error) {
	return splitticket.UtxoMapFromNetwork(dcrd.client, split)
}
