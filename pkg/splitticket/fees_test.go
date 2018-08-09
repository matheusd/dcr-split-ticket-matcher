package splitticket

import (
	"testing"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
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

func TestFeeEstimation(t *testing.T) {
	tx := wire.NewMsgTx()
	maxParts := stake.MaxInputsPerSStx - 1

	voteScript := make([]byte, 1+1+20+1)
	commitScript := make([]byte, 1+1+20+8+2)
	changeScript := make([]byte, 1+1+1+20+1+1)
	nullHash := &chainhash.Hash{}
	fullSigScript := make([]byte, 1+73+1+33)
	txInTempl := wire.NewTxIn(wire.NewOutPoint(nullHash, 0, 0), 0, fullSigScript)
	commitOutTempl := wire.NewTxOut(0, commitScript)
	changeOutTempl := wire.NewTxOut(0, changeScript)
	relayFeeRate := dcrutil.Amount(1e5)

	// vote and pool io
	tx.AddTxIn(txInTempl)
	tx.AddTxOut(wire.NewTxOut(0, voteScript))
	tx.AddTxOut(commitOutTempl)
	tx.AddTxOut(changeOutTempl)

	for i := 1; i <= maxParts; i++ {
		tx.AddTxIn(txInTempl)
		tx.AddTxOut(commitOutTempl)
		tx.AddTxOut(changeOutTempl)

		feeEstimate := SessionFeeEstimate(i)

		txSize := dcrutil.Amount(tx.SerializeSize())
		minFee := (txSize * relayFeeRate) / dcrutil.Amount(1000)
		maxFee := minFee * 101 / 100

		if feeEstimate < minFee {
			t.Fatalf("fee estimate for %d participants (%s) less than minimum "+
				"required (%s - tx size %d)", i, feeEstimate, minFee, txSize)
		} else if feeEstimate > maxFee {
			t.Fatalf("fee estimate for %d participants (%s) more than maximum "+
				"allowed (%s - tx size %d)", i, feeEstimate, maxFee, txSize)
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
