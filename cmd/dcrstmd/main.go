package main

import (
	"fmt"
	"log"
	"os"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/daemon"
)

func main() {
	cfg, err := daemon.LoadConfig()
	if err != nil {
		if err == daemon.ErrHelpRequested {
			return
		} else if err == daemon.ErrVersionRequested {
			fmt.Printf("Split ticket matcher service daemon version %s\n",
				pkg.Version)
			return
		}

		fmt.Println(err)
		os.Exit(1)
	}

	d, err := daemon.NewDaemon(cfg)
	if err != nil {
		panic(err)
	}

	log.Fatal(d.ListenAndServe())
}
