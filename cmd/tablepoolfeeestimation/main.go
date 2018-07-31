package main

import (
	"fmt"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"
)

func main() {
	poolFeeRate := 5.0
	net := &chaincfg.MainNetParams
	blockHeight := 237609
	ticketPrice := dcrutil.Amount(96.47 * 1e8)

	maxParts := stake.MaxInputsPerSStx - 1

	fmt.Printf("Ticket Price: %s\n", ticketPrice)
	fmt.Printf("Block Height: %d\n", blockHeight)
	fmt.Printf("Pool Fee Rate: %.2f%%\n\n", poolFeeRate)

	//                10 |  2.787 | 0.00329 | 0.05965
	fmt.Println("# Parts |   Size |   TxFee | PoolFee")
	for p := 1; p <= maxParts; p++ {
		size := splitticket.TicketSizeEstimate(p)

		ticketFee := splitticket.SessionFeeEstimate(p)

		poolFee := splitticket.SessionPoolFee(p, ticketPrice,
			blockHeight, poolFeeRate, net)

		fmt.Printf("%7d | %6.3f | %.5f | %.5f\n", p,
			float64(size)/1000.0,
			ticketFee.ToUnit(dcrutil.AmountCoin),
			poolFee.ToUnit(dcrutil.AmountCoin))
	}
}
