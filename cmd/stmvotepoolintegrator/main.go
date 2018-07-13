package main

import (
	"fmt"
	"log"
	"os"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg/poolintegrator"
)

func main() {
	cfg, err := poolintegrator.LoadConfig()
	if err != nil {
		if err == poolintegrator.ErrHelpRequested {
			return
		}

		fmt.Println(err)
		os.Exit(1)
	}

	d, err := poolintegrator.NewDaemon(cfg)
	if err != nil {
		panic(err)
	}

	log.Fatal(d.ListenAndServe())
}
