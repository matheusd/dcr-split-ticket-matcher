package main

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg/version"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/buyer"
)

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func main() {
	fmt.Printf("Split ticket buyer version %s\n", version.String())

	if !buyer.DefaultConfigFileExists() {
		fmt.Println("Default config file does not exist. Initializing buyer config based on existing dcrwallet.conf")
		err := buyer.InitConfigFromDcrwallet()
		if err != nil {
			fmt.Println(err)
		} else {
			cfg, err := buyer.LoadConfig()
			if err == nil {
				fmt.Printf("Initialized config file to %s\n", cfg.ConfigFile)
				err = cfg.Validate()
				if err != nil {
					fmt.Printf("Config not yet ready: %s\n", err)
					fmt.Printf("Please edit and complete the config file as needed.\n")
				}
			}
		}
		os.Exit(1)
	}

	cfg, err := buyer.LoadConfig()
	if err == buyer.ErrVersionRequested {
		os.Exit(0)
	}
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

	logDir := path.Join(cfg.DataDir, "logs")
	reporter := buyer.NewWriterReporter(buyer.NewLoggerMiddleware(os.Stdout, logDir),
		cfg.SessionName)
	ctx := context.WithValue(context.Background(), buyer.ReporterCtxKey, reporter)
	ctx, cancelFunc := context.WithCancel(ctx)

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
