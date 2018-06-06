package main

import (
	"os"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"
)

// This is a small utility to generate random data from the lottery hashes,
// in order to check for randomness using dieharder.
func main() {

	chainHash := new(chainhash.Hash)
	err := chainhash.Decode(chainHash, "000000000000437482b6d47f82f374cde539440ddb108b0a76886f0d87d126b9")
	if err != nil {
		panic(err)
	}

	var i int
	var nbs []splitticket.SecretNumber
	var hash []byte

	nbs = []splitticket.SecretNumber{0, 0, 0, 0, 0, 0, 0}

	for {
		for i = 0; i < len(nbs); i++ {
			nbs[i]++
			if nbs[i] != 0 {
				break
			}
		}
		hash = splitticket.CalcLotteryResultHash(nbs, chainHash)
		n, err := os.Stdout.Write(hash)
		if err != nil {
			panic(err)
		}
		if n != 32 {
			panic("wrote too few bytes")
		}
	}

}
