package daemon

import (
	"io/ioutil"
	"time"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/rpcclient"
	"github.com/decred/dcrd/wire"
	logging "github.com/op/go-logging"
)

type DecredNetworkConfig struct {
	Host        string
	User        string
	Pass        string
	CertFile    string
	logBackend  logging.LeveledBackend
	chainParams *chaincfg.Params
}

type DecredNetwork struct {
	client      *rpcclient.Client
	blockHeight int32
	log         *logging.Logger
	ticketPrice uint64
	chainParams *chaincfg.Params
}

func ConnectToDecredNode(cfg *DecredNetworkConfig) (*DecredNetwork, error) {

	log := logging.MustGetLogger("decred-network")
	log.SetBackend(cfg.logBackend)

	net := &DecredNetwork{
		log:         log,
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
	if err := client.NotifyBlocks(); err != nil {
		return nil, err
	}

	err = net.updateFromBestBlock()
	if err != nil {
		return nil, err
	}

	net.log.Noticef("Connected to the decred network. Height=%d StakeDiff=%s", net.blockHeight, dcrutil.Amount(net.ticketPrice))

	go net.maintainClient()

	return net, nil
}

func (net *DecredNetwork) maintainClient() {
	for {
		time.Sleep(30 * time.Second)
		net.log.Debug("Attempting to ping dcrd node")
		err := net.client.Ping()
		if err == nil {
			continue
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

func (net *DecredNetwork) updateFromBestBlock() error {
	bestBlockHash, blockHeight, err := net.client.GetBestBlock()
	if err != nil {
		return err
	}

	bestBlock, err := net.client.GetBlock(bestBlockHash)
	if err != nil {
		return err
	}

	net.ticketPrice = uint64(bestBlock.Header.SBits)
	net.blockHeight = int32(blockHeight)

	return nil
}

func (net *DecredNetwork) notificationHandlers() *rpcclient.NotificationHandlers {
	return &rpcclient.NotificationHandlers{
		OnBlockConnected:    net.onBlockConnected,
		OnBlockDisconnected: net.onBlockDisconnected,
		OnReorganization:    net.onReorganization,
	}
}

func (net *DecredNetwork) onBlockConnected(blockHeader []byte, transactions [][]byte) {
	header := &wire.BlockHeader{}
	header.FromBytes(blockHeader)
	net.ticketPrice = uint64(header.SBits)
	net.blockHeight = int32(header.Height)
	stakeDiffChangeDistance := int32(net.chainParams.WorkDiffWindowSize) -
		(net.blockHeight % int32(net.chainParams.WorkDiffWindowSize))
	net.log.Infof("Block connected. Height=%d StakeDiff=%s WindowChangeDist=%d",
		header.Height, dcrutil.Amount(net.ticketPrice), stakeDiffChangeDistance)
}

func (net *DecredNetwork) onBlockDisconnected(blockHeader []byte) {
	header := &wire.BlockHeader{}
	header.FromBytes(blockHeader)
	net.log.Infof("Block disconnected. Height=%d", header.Height)
	net.updateFromBestBlock()
}

func (net *DecredNetwork) onReorganization(oldHash *chainhash.Hash, oldHeight int32,
	newHash *chainhash.Hash, newHeight int32) {
	net.log.Info("Chain reorg. OldHeight=%d NewHeight=%d", oldHeight, newHeight)
	net.updateFromBestBlock()
}

func (net *DecredNetwork) CurrentTicketPrice() uint64 {
	return net.ticketPrice
}

func (net *DecredNetwork) CurrentBlockHeight() int32 {
	return net.blockHeight
}

func (net *DecredNetwork) ConnectedToDecredNetwork() bool {
	return !net.client.Disconnected()
}
