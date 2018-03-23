package main

import (
	"context"
	"fmt"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg/buyer"
)

func main() {
	reporter := &buyer.StdOutReporter{}
	ctx := context.WithValue(context.Background(), buyer.ReporterCtxKey, reporter)

	maxAmount, _ := dcrutil.NewAmount(60)

	cfg := &buyer.BuyerConfig{
		MatcherHost:    "localhost:8475",
		MaxAmount:      maxAmount,
		SStxFeeLimits:  uint16(0x5800),
		ChainParams:    &chaincfg.TestNet2Params,
		WalletHost:     "localhost:19121",
		WalletCertFile: "/home/user/.config/decrediton/wallets/testnet/default-wallet/rpc.cert",
		VoteAddress:    "TcgmV9RJ9NgWRFGMRGerUdHVGBXK5DJZQnV",
		PoolAddress:    "TsWQ2KCEvE9VWSfk1ywXpT2fQbWLnfV5fDB",
		Passphrase:     []byte("123"),
	}

	err := buyer.BuySplitTicket(ctx, cfg)
	if err != nil {
		fmt.Printf("Error buying split ticket: %v\n", err)
	} else {
		fmt.Printf("Success buying split ticket!\n")
	}
}
