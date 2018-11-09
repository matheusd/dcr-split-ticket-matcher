package splitticket

import (
	"testing"

	"github.com/decred/dcrd/wire"
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

// TestCheckSplitStopsTooManyInputs tests whether a split tx with too many
// inputs is stopped.
func TestCheckSplitStopsTooManyInputs(t *testing.T) {

	data := createStdTestData(20)
	split, _ := data.createTestTransactions()

	for len(split.TxIn) < 30 {
		split.AddTxIn(wire.NewTxIn(&wire.OutPoint{}, 0, nil))
	}

	err := data.checkSplit(split)
	if err == nil {
		t.Fatalf("a split with too many inputs should not be valid")
	}
}
