package main

import (
	"log"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg/daemon"
)

func main() {
	cfg := &(*daemon.DefaultConfig)
	daemon, err := daemon.NewDaemon(cfg)
	if err != nil {
		panic(err)
	}

	log.Fatal(daemon.ListenAndServe())
}
