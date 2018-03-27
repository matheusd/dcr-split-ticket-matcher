package main

import (
	"log"

	"github.com/ansel1/merry"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg/daemon"
)

func main() {
	cfg, err := daemon.LoadConfig()
	if err != nil {
		if merry.Is(err, daemon.ErrHelpRequested, daemon.ErrArgParsingError) {
			return
		}

		panic(err)
	}

	daemon, err := daemon.NewDaemon(cfg)
	if err != nil {
		panic(err)
	}

	log.Fatal(daemon.ListenAndServe())
}
