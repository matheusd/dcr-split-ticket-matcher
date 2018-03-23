package main

import (
	"context"
	"fmt"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg/buyer"
)

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func main() {
	reporter := &buyer.StdOutReporter{}
	ctx := context.WithValue(context.Background(), buyer.ReporterCtxKey, reporter)

	cfg, err := buyer.LoadConfig()
	if err != nil {
		fmt.Printf("Error loading config file: %v\n", err)
		return
	}

	defer func() { zeroBytes(cfg.Passphrase) }()

	err = buyer.BuySplitTicket(ctx, cfg)
	if err != nil {
		fmt.Printf("Error buying split ticket: %v\n", err)
	} else {
		fmt.Printf("Success buying split ticket!\n")
	}
}
