package daemon

import (
	"github.com/decred/slog"
	"io/ioutil"
	"strings"
	"time"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/rpcclient"
	"github.com/decred/dcrd/wire"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type decredNetworkConfig struct {
	Host        string
	User        string
	Pass        string
	CertFile    string
	Log         slog.Logger
	chainParams *chaincfg.Params
}

type decredNetwork struct {
	client      *rpcclient.Client
	blockHeight uint32
	blockHash   chainhash.Hash
	log         slog.Logger
	ticketPrice uint64
	chainParams *chaincfg.Params
}

func connectToDecredNode(cfg *decredNetworkConfig) (*decredNetwork, error) {

	net := &decredNetwork{
		log:         cfg.Log,
		chainParams: cfg.chainParams,
	}

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
	client, err := rpcclient.New(connCfg, net.notificationHandlers())
	if err != nil {
		return nil, err
	}
	net.client = client

	// Register for block connect and disconnect notifications.
	if err = client.NotifyBlocks(); err != nil {
		return nil, err
	}

	nodeNet, err := client.GetCurrentNet()
	if err != nil {
		return nil, err
	}
	nodeNetName := strings.ToLower(nodeNet.String())
	if nodeNetName != cfg.chainParams.Name {
		return nil, errors.Errorf("network of daemon (%s) not the same as the "+
			"expected (%s)", nodeNetName, cfg.chainParams.Name)
	}

	err = net.updateFromBestBlock()
	if err != nil {
		return nil, err
	}

	net.log.Criticalf("Connected to the decred network. Height=%d StakeDiff=%s", net.blockHeight, dcrutil.Amount(net.ticketPrice))

	return net, nil
}

func (net *decredNetwork) run(serverCtx context.Context) {
	var err error
	ticker := time.NewTicker(30 * time.Second)

	for {
		err = nil

		select {
		case <-serverCtx.Done():
			ticker.Stop()
			net.log.Infof("Done daemon network")
			net.client.Shutdown()
			net.client.WaitForShutdown()
			return
		case <-ticker.C:
			net.log.Debug("Trying to ping dcrd")
			pingRespChan := make(chan error)
			go func(c chan error) {
				pingErr := net.client.Ping()
				if pingErr != rpcclient.ErrClientDisconnect {
					c <- pingErr
				}
			}(pingRespChan)

			timeout := time.NewTimer(5 * time.Second)
			select {
			case err = <-pingRespChan:
				if err == nil {
					timeout.Stop()
					net.log.Debug("Pinged dcrd")
					continue
				}
			case <-timeout.C:
				// timeout error. Continue below for reconnect attempt.
				err = errors.New("timeout waiting for ping response")
			}
		}

		net.log.Errorf("Error pinging dcrd: %v", err)
		net.client.Disconnect()
		err = net.updateFromBestBlock()
		if err != nil {
			net.log.Errorf("Error grabing dcrd best block after disconnect: %v", err)
			continue
		}

		net.log.Infof("Reconnected and updated best block to %d StakeDiff %s",
			net.blockHeight, dcrutil.Amount(net.ticketPrice))
	}
}

func (net *decredNetwork) updateFromBestBlock() error {
	bestBlockHash, blockHeight, err := net.client.GetBestBlock()
	if err != nil {
		return err
	}

	bestBlock, err := net.client.GetBlock(bestBlockHash)
	if err != nil {
		return err
	}

	net.ticketPrice = uint64(bestBlock.Header.SBits)
	net.blockHeight = uint32(blockHeight)
	net.blockHash = *bestBlockHash

	return nil
}

func (net *decredNetwork) notificationHandlers() *rpcclient.NotificationHandlers {
	return &rpcclient.NotificationHandlers{
		OnClientConnected:   net.onClientConnected,
		OnBlockConnected:    net.onBlockConnected,
		OnBlockDisconnected: net.onBlockDisconnected,
		OnReorganization:    net.onReorganization,
	}
}

func (net *decredNetwork) onClientConnected() {
	net.log.Infof("Connected to the dcrd daemon")
}

func (net *decredNetwork) onBlockConnected(blockHeader []byte, transactions [][]byte) {
	header := &wire.BlockHeader{}
	header.FromBytes(blockHeader)
	net.ticketPrice = uint64(header.SBits)
	net.blockHeight = header.Height
	net.blockHash = header.BlockHash()
	stakeDiffChangeDistance := splitticket.StakeDiffChangeDistance(net.blockHeight,
		net.chainParams)
	net.log.Infof("Block connected. Height=%d StakeDiff=%s WindowChangeDist=%d",
		header.Height, dcrutil.Amount(net.ticketPrice), stakeDiffChangeDistance)
}

func (net *decredNetwork) onBlockDisconnected(blockHeader []byte) {
	header := &wire.BlockHeader{}
	header.FromBytes(blockHeader)
	net.log.Infof("Block disconnected. Height=%d", header.Height)
	net.updateFromBestBlock()
}

func (net *decredNetwork) onReorganization(oldHash *chainhash.Hash, oldHeight int32,
	newHash *chainhash.Hash, newHeight int32) {
	net.log.Infof("Chain reorg. OldHeight=%d NewHeight=%d", oldHeight, newHeight)
	net.updateFromBestBlock()
}

func (net *decredNetwork) CurrentTicketPrice() uint64 {
	return net.ticketPrice
}

func (net *decredNetwork) CurrentBlockHeight() uint32 {
	return net.blockHeight
}

func (net *decredNetwork) CurrentBlockHash() chainhash.Hash {
	return net.blockHash
}

func (net *decredNetwork) ConnectedToDecredNetwork() bool {
	return !net.client.Disconnected()
}

func (net *decredNetwork) PublishTransactions(txs []*wire.MsgTx) error {
	for i, tx := range txs {
		_, err := net.client.SendRawTransaction(tx, false)
		if err != nil {
			return errors.Wrapf(err, "error publishing tx %d", i)
		}
	}
	return nil
}

func (net *decredNetwork) GetUtxos(outpoints []*wire.OutPoint) (
	splitticket.UtxoMap, error) {
	return splitticket.UtxoMapOutpointsFromNetwork(net.client, outpoints)
}
