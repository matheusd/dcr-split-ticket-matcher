// sessionsstatus lists all sessions currently in a dcrstmd instance dir (by
// default ~/.dcrstmd/sessions) and gets the state of the tickets by consulting
// with the locally running dcrd instance
package main

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/rpcclient"
	"github.com/decred/dcrd/wire"
	flags "github.com/jessevdk/go-flags"
)

var (
	currentBlock int64
	chainParams  *chaincfg.Params
)

func orPanic(err error) {
	if err != nil {
		panic(err)
	}
}

type config struct {
	RPCServer   string `short:"s" long:"rpcserver" description:"Address of the dcrd daemon"`
	RPCUser     string `short:"u" long:"rpcuser" description:"RPC user to connect to dcrd"`
	RPCPass     string `short:"P" long:"rpcpass" description:"RPC password to connect to dcrd"`
	RPCCert     string `short:"c" long:"rpccert" description:"RPC certificate location"`
	TestNet     bool   `long:"testnet" description:"Whether to connect to a testnet host"`
	SessionsDir string `short:"d" long:"sessionsdir" description:"Path to the sessions dir of dcrstmd"`
}

func readConfig() *config {
	cfg := &config{
		RPCUser:     "USER",
		RPCPass:     "PASSWORD",
		RPCServer:   "",
		RPCCert:     path.Join(dcrutil.AppDataDir("dcrd", false), "rpc.cert"),
		SessionsDir: path.Join(dcrutil.AppDataDir("dcrstmd", false), "sessions"),
	}

	parser := flags.NewParser(cfg, flags.Default)
	_, err := parser.Parse()
	if err != nil {
		e, ok := err.(*flags.Error)
		if ok && e.Type == flags.ErrHelp {
			os.Exit(0)
		}
		fmt.Printf("Command Line Parsing Error: %v\n", err)
		parser.WriteHelp(os.Stderr)
		os.Exit(1)
	}

	if cfg.RPCServer == "" {
		if cfg.TestNet {
			cfg.RPCServer = "127.0.0.1:19109"
		} else {
			cfg.RPCServer = "127.0.0.1:9109"
		}
	}

	return cfg
}

func connectToDcrd(cfg *config) (*rpcclient.Client, error) {
	certs, err := ioutil.ReadFile(cfg.RPCCert)
	if err != nil {
		log.Fatal(err)
	}
	connCfg := &rpcclient.ConnConfig{
		Host:         cfg.RPCServer,
		Endpoint:     "ws",
		User:         cfg.RPCUser,
		Pass:         cfg.RPCPass,
		Certificates: certs,
	}
	client, err := rpcclient.New(connCfg, nil)
	return client, err
}

type ticketInfo struct {
	status      string
	minedHeight int64
}

func getTicketInfo(txHashStr string, client *rpcclient.Client) *ticketInfo {
	txHash, err := chainhash.NewHashFromStr(txHashStr)
	orPanic(err)

	res := &ticketInfo{
		status: "unknown",
	}

	tx, err := client.GetRawTransactionVerbose(txHash)
	if err != nil && strings.Contains(err.Error(), "No information available") {
		return res
	}
	orPanic(err)

	txBytes, err := hex.DecodeString(tx.Hex)
	orPanic(err)
	msgTx := wire.NewMsgTx()
	msgTx.FromBytes(txBytes)

	res.minedHeight = tx.BlockHeight

	if !stake.IsSStx(msgTx) {
		res.status = "not ticket"
		return res
	}

	if tx.BlockHeight <= 0 {
		res.status = "unmined"
	} else if currentBlock-tx.BlockHeight < int64(chainParams.TicketMaturity) {
		res.status = "immature"
	} else {
		resMissed, err := client.ExistsMissedTickets([]*chainhash.Hash{txHash})
		orPanic(err)

		resExpired, err := client.ExistsExpiredTickets([]*chainhash.Hash{txHash})
		orPanic(err)

		resLive, err := client.ExistsLiveTicket(txHash)
		orPanic(err)

		if resMissed != "00" {
			res.status = "missed"
		} else if resExpired != "00" {
			res.status = "expired"
		} else if resLive {
			res.status = "live"
		} else {
			res.status = "spent"
		}
	}

	return res
}

func main() {
	cfg := readConfig()
	chainParams = &chaincfg.MainNetParams
	if cfg.TestNet {
		chainParams = &chaincfg.TestNet3Params
	}

	client, err := connectToDcrd(cfg)
	orPanic(err)

	_, currentBlock, err = client.GetBestBlock()
	orPanic(err)

	files, err := ioutil.ReadDir(cfg.SessionsDir)
	orPanic(err)

	totals := map[string]int{
		"not ticket": 0,
		"unmined":    0,
		"immature":   0,
		"missed":     0,
		"expired":    0,
		"live":       0,
		"spent":      0,
	}

	//   66660b757bef6d9089a742c320c00009d49f54efda02380766c85f8cfe757eac      unknown        0
	fmt.Printf("                                                            hash       status    mined\n")
	for _, f := range files {
		txHashStr := f.Name()
		tf := getTicketInfo(txHashStr, client)
		fmt.Printf("%s  %11s  %7d\n", txHashStr, tf.status, tf.minedHeight)
		totals[tf.status]++
	}

	fmt.Println("")
	fmt.Printf("Current block height: %d\n", currentBlock)
	fmt.Printf("Totals:\n")
	i := 0
	for k, v := range totals {
		fmt.Printf("%11s: %4d     ", k, v)
		i++
		if i%3 == 0 {
			fmt.Printf("\n")
		}
	}
	fmt.Printf("\n\n")
}
