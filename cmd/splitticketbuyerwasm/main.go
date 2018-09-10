package main

import (
	"context"
	"fmt"
	"os"
	"syscall/js"
	"time"

	"github.com/decred/dcrd/chaincfg"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/buyer"
)

var (
	done chan struct{}
	buy chan struct{}

	cfg = &buyer.Config{
		SStxFeeLimits:        uint16(0x5800),
		ChainParams:          &chaincfg.MainNetParams,
		SourceAccount:        0,
		MaxTime:              30,
		MaxWaitTime:          0,
		SkipWaitPublishedTxs: true,
		PoolFeeRate:          5.0,

		MatcherHost: "localhost:8475",
		MatcherCertFile: "/home/user/.dcrstmd/rpc.cert",
		VoteAddress: "TcgV6Kuv1NUbQwXGcr1QN3voNv1SjDnbfbo",
		PoolAddress: "TsUD8an3ws48UEXZTT3sXjpEgZ6krCDX9CQ",
		TestNet: true,
		MaxAmount: 2,
		Passphrase: []byte("123"),
		DcrdHost: "localhost:19119",
		DcrdUser: "USER",
		DcrdPass: "PASSWORD",
		DcrdCert: "/home/user/.dcrd/rpc.cert",
		WalletHost: "localhost:19121",
		//WalletCertFile: "/home/user/.config/decrediton/wallets/testnet/default-wallet/rpc.cert",
		WalletCert: `
-----BEGIN CERTIFICATE-----
MIIB1jCCAX2gAwIBAgIRAPJBkJuMIgxY0mDGIxiOVu4wCgYIKoZIzj0EAwIwNTEl
MCMGA1UEChMcZGNyd2FsbGV0IGF1dG9nZW5lcmF0ZWQgY2VydDEMMAoGA1UEAxMD
ZGV2MB4XDTE4MDkwNTE3MzMyNFoXDTI4MDkwMzE3MzMyNFowNTElMCMGA1UEChMc
ZGNyd2FsbGV0IGF1dG9nZW5lcmF0ZWQgY2VydDEMMAoGA1UEAxMDZGV2MFkwEwYH
KoZIzj0CAQYIKoZIzj0DAQcDQgAEwHlW6igg+zaGkj1l7naEsaXdtAmvrJGSrJcS
JbSfKUMlNZueNtAVMAZAGqo0kSyr3+xL3olwcn4SvmiaVFllIaNuMGwwDgYDVR0P
AQH/BAQDAgKkMA8GA1UdEwEB/wQFMAMBAf8wSQYDVR0RBEIwQIIDZGV2gglsb2Nh
bGhvc3SHBH8AAAGHEAAAAAAAAAAAAAAAAAAAAAGHBAqJAg+HEP6AAAAAAAAAAhY+
//5ebA0wCgYIKoZIzj0EAwIDRwAwRAIgCqnIh7vBXoNyL1KOb4G8fQHpcbQwoQ6T
1niJeOTT0IoCIGhAp7FWOITL1szglJrFiv23fQW1/xrrdtopqg7nyqJM
-----END CERTIFICATE-----
		`,
	}
)

func debug(format string, args... interface{}) {
	format = time.Now().Format("15:04:05.000 ") + format
	println(fmt.Sprintf(format, args...))
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func buySplitTicket(args []js.Value) {
	buy<- struct{}{}
}

func startBuyer() {
	err := cfg.Validate()
	if err != nil {
		fmt.Printf("Error validating config: %v\n", err)
		return
	}
	// defer func() { zeroBytes(cfg.Passphrase) }()

	adapter := newWasmMatcherClientAdapter()
	cfg.MatcherClient = adapter

	cbl := js.NewCallback(adapter.callback)
	js.Global().Set("_splitTicketBuyer_matcherClient_callback", cbl)
	defer cbl.Release()

	reporter := buyer.NewWriterReporter(os.Stdout, cfg.SessionName)
	ctx := context.WithValue(context.Background(), buyer.ReporterCtxKey, reporter)
	ctx, cancelFunc := context.WithCancel(ctx)

	// go buyer.WatchMatcherWaitingList(ctx, cfg.MatcherHost, cfg.MatcherCertFile,
	// 	reporter)

	err = buyer.BuySplitTicket(ctx, cfg)
	if err != nil {
		fmt.Printf("Error buying split ticket: %v\n", err)
	} else {
		fmt.Printf("Success buying split ticket!\n")
	}

	cancelFunc()

	done<- struct{}{}
}

func main() {
	done = make(chan struct{})
	buy = make(chan struct{})

	fmt.Printf("Split ticket buyer version %s\n", pkg.Version)

	cbl := js.NewCallback(buySplitTicket)
	js.Global().Set("_buySplitTicket", cbl)

	select {
	case <-done:
	case <-buy:
		debug("received buy signal")
		go startBuyer()
		<-done
		debug("received done signal")
		time.Sleep(13*time.Second)
	}

	close(done)
	close(buy)

	debug("Going to quit...")

	// cbl.Release()
}
