package main

import (
	"fmt"
	"sync"

	"github.com/decred/dcrd/blockchain/stake"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"
)

var net = &chaincfg.MainNetParams

func calcMinVoteReward(ticketHeight int64) dcrutil.Amount {
	var initCache sync.Once
	var subsidyCache *blockchain.SubsidyCache

	maxVoteHeight := ticketHeight + int64(net.TicketMaturity) + int64(net.TicketExpiry)
	initCache.Do(func() {
		subsidyCache = blockchain.NewSubsidyCache(maxVoteHeight, net)
	})

	subsidy := blockchain.CalcStakeVoteSubsidy(subsidyCache, maxVoteHeight, net)

	return dcrutil.Amount(subsidy)
}

func main() {
	// Tweak these paramters to simulate a given scenario
	poolFeeRate := 5.0                          // fee rate percentage for the pool
	blockHeight := 315000                       // block height where ticket is purchased
	ticketPrice := dcrutil.Amount(120 * 1e8)    // ticket price
	minPartReward, _ := dcrutil.NewAmount(0.05) // how many coins from the reward the minimum participant should receive

	// from here on, everything is calculated given the above parameters

	maxPossibleParts := stake.MaxInputsPerSStx - 1
	minReward := calcMinVoteReward(int64(blockHeight))
	minPartAmount := ticketPrice * minPartReward / minReward
	maxParts := int(ticketPrice / minPartAmount)
	if maxParts > maxPossibleParts {
		maxParts = maxPossibleParts
	}
	maxPoolFee := splitticket.SessionPoolFee(maxParts, ticketPrice,
		blockHeight, poolFeeRate, net)
	minPartPoolFee := minPartAmount * maxPoolFee / ticketPrice

	fmt.Printf("Ticket Price: %s\n", ticketPrice)
	fmt.Printf("Block Height: %d\n", blockHeight)
	fmt.Printf("Pool Fee Rate: %.2f%%\n", poolFeeRate)
	fmt.Printf("Minimum future reward: %s\n", minReward)
	fmt.Printf("Minimum participation amount: %s\n", minPartAmount)
	fmt.Printf("Pool fee at minimum participation: %s\n", minPartPoolFee)

	//                10 |  2.787 | 0.00329 | 0.05965
	fmt.Println("\n# Parts |   Size |   TxFee | PoolFee | PartFee | MinPrft")
	for p := 1; p <= maxParts; p++ {
		size := splitticket.TicketSizeEstimate(p)

		ticketFee := splitticket.SessionFeeEstimate(p)

		poolFee := splitticket.SessionPoolFee(p, ticketPrice,
			blockHeight, poolFeeRate, net)

		partTxFee := splitticket.SessionParticipantFee(p)

		profitMinPart := minPartReward - partTxFee - minPartPoolFee

		fmt.Printf("%7d | %6.3f | %.5f | %.5f | %.5f | %.5f\n", p,
			float64(size)/1000.0,
			ticketFee.ToUnit(dcrutil.AmountCoin),
			poolFee.ToUnit(dcrutil.AmountCoin),
			partTxFee.ToUnit(dcrutil.AmountCoin),
			profitMinPart.ToUnit(dcrutil.AmountCoin))
	}
}
