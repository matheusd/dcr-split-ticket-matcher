package buyer

import (
	"io/ioutil"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/rpcclient"
)

type decredNetworkConfig struct {
	Host        string
	User        string
	Pass        string
	CertFile    string
	chainParams *chaincfg.Params
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
		Host:         cfg.Host,
		Endpoint:     "ws",
		User:         cfg.User,
		Pass:         cfg.Pass,
		Certificates: certs,
	}
	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		return nil, err
	}

	net.client = client

	return net, nil
}
