package splitticket

import (
	"testing"
)

// TestCheckSplitStopsWrongValueIn checks whether CheckSignedSplit picks up
// changes to ValueIn
func TestCheckSplitStopsWrongValueIn(t *testing.T) {

	data := createStdTestData(20)
	split, ticket := data.createTestTransactions()
	split.TxIn[1].ValueIn--
	data.signTicket(split, ticket)

	err := data.checkSignedSplit(split)
	if err == nil {
		t.Fatalf("changing the valueIn of the split tx should have returned " +
			"an error")
	}
}
