package splitticket

import (
	"fmt"
	"testing"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/dcrutil"
)

// TestStopsWrongTicketFees tests whether various combinations of number of
// participants and fee rates that could be provided by a matcher are caught by
// the validation functions
func TestStopsWrongTicketFees(t *testing.T) {
	t.Parallel()

	ticketPrice := dcrutil.Amount(200 * dcrutil.AtomsPerCoin)

	testFeeFunc := func(nbParts int, fee dcrutil.Amount) func(t *testing.T) {
		return func(t *testing.T) {
			t.Parallel()
			data := createBaseTestData(nbParts)
			data.fillParticipationData(ticketPrice, fee, 5.0)
			data.fillAddressesAndKeys()
			split, ticket := data.createTestTransactions()
			data.signTicket(split, ticket)
			err := data.checkSignedTicket(split, ticket)
			if err == nil {
				t.Fatalf("Ticket with %d parts and tx fee %s should be "+
					"detected by CheckSignedTicket()", nbParts, fee)
			}
		}
	}

	testPartFunc := func(nbParts int) func(t *testing.T) {
		return func(t *testing.T) {
			t.Parallel()
			acceptableFee := SessionParticipantFee(nbParts)
			partFeesToTest := []dcrutil.Amount{
				0, 1, 10, 100, 1000, 10000, // very small fees
				1000000, 10000000, 100000000, 1000000000, // very high fees
				acceptableFee - 80000, // slight less than needed
				acceptableFee*2 + 30,  // slightly more than allowed
			}
			for _, fee := range partFeesToTest {
				t.Run(fmt.Sprintf("%d", fee), testFeeFunc(nbParts, fee))
			}
		}
	}

	maxParts := stake.MaxInputsPerSStx - 1
	for i := 1; i < maxParts; i++ {
		t.Run(fmt.Sprintf("%d", i), testPartFunc(i))
	}
}

// TestStopsWrongPoolFees tests whether various combinations of number of
// participants and poo fees that could be provided by a matcher are caught by
// the validation functions
func TestStopsWrongPoolFees(t *testing.T) {

	ticketPrice := dcrutil.Amount(200 * dcrutil.AtomsPerCoin)

	testFeeFunc := func(nbParts int, poolFeeRate float64) func(t *testing.T) {
		return func(t *testing.T) {
			t.Parallel()

			// by changing the poolFeeRate with a delta, we're assuming a
			// participant has recorded that the pool uses poolFeeRate
			// but that the service actually used poolFeeRate+delta to create the
			// transactions. This then tests whether the participant detects
			// that the service used a pool fee higher than he had agreed to.
			delta := 0.16

			partFee := SessionParticipantFee(nbParts)
			data := createBaseTestData(nbParts)
			data.fillParticipationData(ticketPrice, partFee, poolFeeRate+delta)
			data.fillAddressesAndKeys()
			split, ticket := data.createTestTransactions()
			data.signTicket(split, ticket)

			err := CheckTicketPoolFeeRate(split, ticket, poolFeeRate,
				data.currentBlockHeight, _testNetwork)
			if err == nil {
				t.Fatalf("Ticket with %d parts and pool fee rate %f and "+
					"delta %f should be detected by CheckTicketPoolFeeRate()", nbParts,
					poolFeeRate, delta)
			}
		}
	}
	testPartFunc := func(nbParts int) func(t *testing.T) {
		return func(t *testing.T) {
			t.Parallel()
			poolFeeRatesToTest := []float64{
				0.02, 0.1, 1, 4.9, 5, 7.49, 7.5, 10,
			}
			for _, fee := range poolFeeRatesToTest {
				t.Run(fmt.Sprintf("%f", fee), testFeeFunc(nbParts, fee))
			}
		}
	}

	maxParts := stake.MaxInputsPerSStx - 1
	for i := 1; i < maxParts; i++ {
		t.Run(fmt.Sprintf("%d", i), testPartFunc(i))
	}
}

// TestCheckTicketStopsWrongValueIn tests whether CheckTicket identifies a
// broken valueIn
func TestCheckTicketStopsWrongValueIn(t *testing.T) {

	data := createStdTestData(20)
	split, ticket := data.createTestTransactions()
	ticket.TxIn[1].ValueIn -= 1
	data.signTicket(split, ticket)

	err := data.checkTicket(split, ticket)
	if err == nil {
		t.Fatalf("changing the valueIn of the ticket should have returned an "+
			"error")
	}
}
