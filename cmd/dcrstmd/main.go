package main

import (
	"fmt"
	"log"
	"os"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg/daemon"
)

func main() {
	cfg, err := daemon.LoadConfig()
	if err != nil {
		if err == daemon.ErrHelpRequested {
			return
		}

		fmt.Println(err)
		os.Exit(1)
	}

	daemon, err := daemon.NewDaemon(cfg)
	if err != nil {
		panic(err)
	}

	log.Fatal(daemon.ListenAndServe())
}
