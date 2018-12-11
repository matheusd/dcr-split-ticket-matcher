package buyer

import (
	"context"
	"github.com/pkg/errors"
	"io/ioutil"
	"time"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"

	"github.com/decred/dcrd/rpcclient"
	"github.com/decred/dcrd/wire"
)

type decredNetworkConfig struct {
	Host     string
	User     string
	Pass     string
	CertFile string
}

type decredNetwork struct {
	client *rpcclient.Client
}

func connectToDecredNode(cfg *decredNetworkConfig) (*decredNetwork, error) {

	net := &decredNetwork{}

	// Connect to local dcrd RPC server using websockets.
	certs, err := ioutil.ReadFile(cfg.CertFile)
	if err != nil {
		return nil, err
	}
	connCfg := &rpcclient.ConnConfig{
		Host:                 cfg.Host,
		Endpoint:             "ws",
		User:                 cfg.User,
		Pass:                 cfg.Pass,
		Certificates:         certs,
		DisableAutoReconnect: true,
	}

	ntfsHandler := &rpcclient.NotificationHandlers{}

	client, err := rpcclient.New(connCfg, ntfsHandler)
	if err != nil {
		return nil, err
	}

	net.client = client

	return net, nil
}

// checkDcrdWaitingForSession repeatedly pings the dcrd node while the
// buyer is waiting for a session, so that if the node is closed the buyer is
// alerted about this fact.
// If context is canceled, then this returns nil.
// This blocks, therefore it MUST be run from a goroutine.
func (dcrd *decredNetwork) checkDcrdWaitingForSession(waitCtx context.Context) error {
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			return nil
		case <-ticker.C:
			err := dcrd.client.Ping()
			if err != nil {
				return errors.Wrap(err, "error pinging dcrd node")
			}
		}
	}
}

func (dcrd *decredNetwork) fetchSplitUtxos(split *wire.MsgTx) (splitticket.UtxoMap, error) {
	return splitticket.UtxoMapFromNetwork(dcrd.client, split)
}
