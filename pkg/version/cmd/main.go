package main

import (
	"fmt"
	"os"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg/version"
)

func main() {
	if len(os.Args) > 1 {
		if os.Args[1] == "release" {
			fmt.Println(version.Root())
			return
		}
	}

	fmt.Println(version.String())
}
