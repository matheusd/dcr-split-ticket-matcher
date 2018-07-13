package poolintegrator

import (
	"io/ioutil"

	"github.com/decred/dcrd/rpcclient"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/internal/util"
	"github.com/pkg/errors"
)

// connectToDcrWallet tries to connect to the given wallet.
func connectToDcrWallet(cfg *Config) (*rpcclient.Client, error) {

	certFile := util.CleanAndExpandPath(cfg.DcrwCert)
	certs, err := ioutil.ReadFile(certFile)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading dcrwcert %s", certFile)
	}

	connCfg := &rpcclient.ConnConfig{
		Host:         cfg.DcrwHost,
		Endpoint:     "ws",
		User:         cfg.DcrwUser,
		Pass:         cfg.DcrwPass,
		Certificates: certs,
	}

	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error creating connection to wallet")
	}

	_, err = client.GetAccountAddress("default")
	if err != nil {
		return nil, errors.Wrap(err, "error trying to get test address from wallet")
	}

	return client, nil
}
