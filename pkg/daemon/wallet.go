package daemon

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/decred/dcrd/dcrjson"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/rpcclient"
	"github.com/decred/dcrd/wire"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/util"
	logging "github.com/op/go-logging"
)

type WalletClient struct {
	log            *logging.Logger
	client         *rpcclient.Client
	poolFeeAddress dcrutil.Address
}

func NewWalletClient() *WalletClient {

	w := &WalletClient{
		log: logging.MustGetLogger("dcrwallet client"),
	}

	util.SetLoggerBackend(true, "", "", logging.INFO, w.log)

	w.connectToDcrWallet()

	return w
}

func (wallet *WalletClient) connectToDcrWallet() {
	dcrwalletHomeDir := dcrutil.AppDataDir("dcrwallet", false)
	certs, err := ioutil.ReadFile(filepath.Join(dcrwalletHomeDir, "ticket-split-wallet", "rpc.cert"))
	if err != nil {
		wallet.log.Fatalf("Error reading dcrwallet cert: %v", err)
	}
	connCfg := &rpcclient.ConnConfig{
		Host:         "localhost:19229",
		Endpoint:     "ws",
		User:         "USER",
		Pass:         "PASSWORD",
		Certificates: certs,
	}

	// TODO: handle client not connecting to daemon
	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		wallet.log.Fatalf("Error creating new rpc client: %v", err)
	}

	addr, err := client.GetAccountAddress("default")
	if err != nil {
		panic(err)
	}

	wallet.client = client

	wallet.log.Infof("Account address %s", addr)
	wallet.poolFeeAddress = addr
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
		return nil, ErrPoolFeeInputNotSigned
	}

	return signed.TxIn[0].SignatureScript, nil
}
