package splitticket

import (
	"testing"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/dcrutil"
)

func TestEqualTxFees(t *testing.T) {
	maxParts := stake.MaxInputsPerSStx - 1

	for p := 1; p <= maxParts; p++ {
		fee := SessionFeeEstimate(p)
		if (fee % dcrutil.Amount(p)) != 0 {
			t.Errorf("Session fee for %d participants is not equally divided", p)
		}
	}
}

func TestCheckParticipantSessionPoolFee(t *testing.T) {

	nbParts := 63
	ticketPrice, _ := dcrutil.NewAmount(100)
	blockHeight := 260000
	poolFeeRate := 5.0

	totalPoolFee := SessionPoolFee(nbParts, ticketPrice, blockHeight, poolFeeRate,
		_testNetwork)

	contribAmount := dcrutil.Amount(float64(ticketPrice) * 0.1)
	partPoolFee := dcrutil.Amount(float64(totalPoolFee) * 0.1)
	partFee, _ := dcrutil.NewAmount(0.001)

	err := CheckParticipantSessionPoolFee(nbParts, ticketPrice, contribAmount,
		partPoolFee, partFee, blockHeight, poolFeeRate, _testNetwork)
	if err != nil {
		t.Fatalf("Correct participant pool fee should not return the following "+
			"error: %v", err)
	}

	partPoolFee += 1000
	err = CheckParticipantSessionPoolFee(nbParts, ticketPrice, contribAmount,
		partPoolFee, partFee, blockHeight, poolFeeRate, _testNetwork)
	if err == nil {
		t.Fatalf("Pool fee higher than contribution amount should have " +
			"returned an error")
	}

}
