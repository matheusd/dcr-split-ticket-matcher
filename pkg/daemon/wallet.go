package daemon

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"

	"github.com/decred/dcrd/dcrjson"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/rpcclient"
	"github.com/decred/dcrd/wire"
	logging "github.com/op/go-logging"
	"github.com/pkg/errors"
)

type WalletConfig struct {
	Host     string
	User     string
	Pass     string
	CertFile string

	logBackend logging.LeveledBackend
}

type WalletClient struct {
	log            *logging.Logger
	client         *rpcclient.Client
	poolFeeAddress dcrutil.Address
}

func ConnectToDcrWallet(cfg *WalletConfig) (*WalletClient, error) {

	w := &WalletClient{
		log: logging.MustGetLogger("dcrwallet client"),
	}

	w.log.SetBackend(cfg.logBackend)

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

	// TODO: handle client not connecting to daemon
	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		return nil, err
	}

	addr, err := client.GetAccountAddress("default")
	if err != nil {
		return nil, err
	}

	w.client = client

	w.log.Infof("Account address %s", addr)
	w.poolFeeAddress = addr

	return w, nil
}

func (wallet *WalletClient) SignRevocation(ticket, revocation *wire.MsgTx) (*wire.MsgTx, error) {
	inputs := make([]dcrjson.RawTxInput, len(ticket.TxOut))
	txid := ticket.TxHash().String()
	for i, out := range ticket.TxOut {
		inputs[i] = dcrjson.RawTxInput{
			Txid:         txid,
			Vout:         uint32(i),
			Tree:         wire.TxTreeStake,
			ScriptPubKey: hex.EncodeToString(out.PkScript),
		}
	}
	signed, all, err := wallet.client.SignRawTransaction2(revocation, inputs)
	if !all && err == nil {
		return nil, fmt.Errorf("Not all inputs for the revocation were signed")
	}
	return signed, err
}

func (wallet *WalletClient) PoolFeeAddress() dcrutil.Address {
	return wallet.poolFeeAddress
}

func (wallet *WalletClient) SignPoolSplitOutput(split, ticket *wire.MsgTx) ([]byte, error) {

	inputs := make([]dcrjson.RawTxInput, len(split.TxOut))
	txid := split.TxHash().String()
	for i, out := range split.TxOut {
		inputs[i] = dcrjson.RawTxInput{
			Txid:         txid,
			Vout:         uint32(i),
			Tree:         wire.TxTreeRegular,
			ScriptPubKey: hex.EncodeToString(out.PkScript),
		}
	}
	signed, _, err := wallet.client.SignRawTransaction2(ticket, inputs)
	if err != nil {
		return nil, err
	}

	if signed.TxIn[0].SignatureScript == nil {
		return nil, errors.Errorf("pool fee input is not signed")
	}

	return signed.TxIn[0].SignatureScript, nil
}

// ValidateVoteAddress checks whether the connected wallet can sign
// transactions with the provided addresses
func (wallet *WalletClient) ValidateVoteAddress(voteAddr dcrutil.Address) error {

	voteAddrStr := voteAddr.EncodeAddress()

	resp, err := wallet.client.ValidateAddress(voteAddr)
	if err != nil {
		return errors.Wrapf(err, "error validating vote address %s", voteAddrStr)
	}

	if !resp.IsValid {
		return errors.Errorf("vote address is invalid: %s", voteAddrStr)
	}

	if !resp.IsMine {
		return errors.Errorf("vote address not controlled by matcher: %s", voteAddrStr)
	}

	return nil

}
