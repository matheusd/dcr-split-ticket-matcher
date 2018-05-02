package main

import (
	"context"
	"fmt"
	"os"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/buyer"
)

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func main() {
	fmt.Printf("Split ticket buyer version %s\n", pkg.Version)

	if !buyer.DefaultConfigFileExists() {
		fmt.Println("Initializing buyer config based on existing dcrwallet.conf")
		err := buyer.InitConfigFromDcrwallet()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	reporter := buyer.NewWriterReporter(os.Stdout)
	ctx := context.WithValue(context.Background(), buyer.ReporterCtxKey, reporter)
	ctx, cancelFunc := context.WithCancel(ctx)

	cfg, err := buyer.LoadConfig()
	if err == nil {
		err = cfg.ReadPassphrase()
	}
	if err == nil {
		err = cfg.Validate()
	}
	if err != nil {
		fmt.Printf("Error loading config file: %v\n", err)
		os.Exit(1)
	}
	defer func() { zeroBytes(cfg.Passphrase) }()

	go buyer.WatchMatcherWaitingList(ctx, cfg.MatcherHost, cfg.MatcherCertFile,
		reporter)

	err = buyer.BuySplitTicket(ctx, cfg)
	if err != nil {
		fmt.Printf("Error buying split ticket: %v\n", err)
	} else {
		fmt.Printf("Success buying split ticket!\n")
	}

	cancelFunc()
}
