package buyer

import (
	"encoding/json"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/wire"
	dcrdatatypes "github.com/decred/dcrdata/api/types"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"
	"github.com/pkg/errors"
	"net/http"
	"time"
)

// utxoProviderForDcrdataURL returns a UtxoMapProvider function that fetches
// utxo information from the given dcrdata URL.
func utxoProviderForDcrdataURL(dcrdataURL string) utxoMapProvider {
	return func(tx *wire.MsgTx) (splitticket.UtxoMap, error) {
		return splitticket.UtxoMapFromDcrdata(dcrdataURL, tx)
	}
}

// isDcrdataOnline checks whether there is a dcrdata online at the given URL and
// that it is for the given network. Returns nil if successful or an error.
func isDcrdataOnline(dcrdataURL string, chainParams *chaincfg.Params) error {
	url := dcrdataURL + "/api/status"
	client := http.Client{Timeout: time.Second * 10}
	urlResp, err := client.Get(url)
	if err != nil {
		return errors.Wrap(err, "error during GET /api/status call")
	}

	defer urlResp.Body.Close()
	dec := json.NewDecoder(urlResp.Body)
	resp := new(dcrdatatypes.Status)
	err = dec.Decode(&resp)
	if err != nil {
		return errors.Wrap(err, "error decoding json response")
	}

	if !resp.Ready {
		return errors.New("dcrdata instance not ready for use")
	}

	// TODO: check if network is the same as the one in chainParams (see issue
	// decred/dcrdata#800)

	return nil
}
