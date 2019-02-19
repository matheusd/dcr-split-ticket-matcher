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

// TestCheckOnlySignedInSplit tests whether the CheckOnlySignedInSplit()
// function behaves as expected.
func TestCheckOnlySignedInSplit(t *testing.T) {
	// Create a dummy split tx with the required data for the tests.
	tx := wire.MsgTx{}

	// Test Outpoints.
	var outp0, outp1, outp2, outp3 wire.OutPoint
	outp1.Hash[0] = 0x01
	outp2.Hash[0] = 0x02
	outp3.Hash[0] = 0x03

	testSigScript := []byte{1, 2, 3}

	tx.AddTxIn(wire.NewTxIn(&outp0, 0, nil))
	tx.AddTxIn(wire.NewTxIn(&outp1, 0, nil))
	tx.AddTxIn(wire.NewTxIn(&outp2, 0, nil))

	testCases := []struct {
		name       string
		sigScripts [][]byte
		outpoints  []wire.OutPoint
		valid      bool
	}{
		{
			name:       "no expected outpoints returns valid",
			sigScripts: [][]byte{nil, nil, nil},
			outpoints:  []wire.OutPoint{},
			valid:      true,
		},
		{
			name:       "requested and signed all outpoints returns valid",
			sigScripts: [][]byte{testSigScript, testSigScript, testSigScript},
			outpoints:  []wire.OutPoint{outp0, outp1, outp2},
			valid:      true,
		},
		{
			name:       "requested and signed only one outpoint",
			sigScripts: [][]byte{testSigScript, nil, nil},
			outpoints:  []wire.OutPoint{outp0},
			valid:      true,
		},
		{
			name:       "requested and signed outpoint order doesn't matter",
			sigScripts: [][]byte{testSigScript, nil, testSigScript},
			outpoints:  []wire.OutPoint{outp2, outp0},
			valid:      true,
		},
		{
			name:       "signing wrong outputoint returns error",
			sigScripts: [][]byte{testSigScript, nil, nil},
			outpoints:  []wire.OutPoint{outp1},
			valid:      false,
		},
		{
			name:       "signing less than the number of requested outpoints returns errors",
			sigScripts: [][]byte{testSigScript, nil, nil},
			outpoints:  []wire.OutPoint{outp0, outp1},
			valid:      false,
		},
		{
			name:       "signing more than the number of requested outpoints returns errors",
			sigScripts: [][]byte{testSigScript, testSigScript, nil},
			outpoints:  []wire.OutPoint{outp0},
			valid:      false,
		},
		{
			name:       "signing no outpoints returns error",
			sigScripts: [][]byte{testSigScript, nil, testSigScript},
			outpoints:  []wire.OutPoint{},
			valid:      false,
		},
		{
			name:       "requesting an outpoint not from the tx returns an error",
			sigScripts: [][]byte{nil, nil, nil},
			outpoints:  []wire.OutPoint{outp3},
			valid:      false,
		},
		{
			name:       "using empty sigscript instead of nil when not requested is valid",
			sigScripts: [][]byte{{}, testSigScript, nil},
			outpoints:  []wire.OutPoint{outp1},
			valid:      true,
		},
		{
			name:       "using empty sigscript instead of nil when requested returns an error",
			sigScripts: [][]byte{{}, nil, nil},
			outpoints:  []wire.OutPoint{outp0},
			valid:      false,
		},
	}

	for _, tc := range testCases {

		// Fill the test cases' sig script.
		for i, sigScript := range tc.sigScripts {
			tx.TxIn[i].SignatureScript = sigScript
		}

		// Execute the function under test and check the result.
		err := CheckOnlySignedInSplit(&tx, tc.outpoints)

		if (err != nil) && tc.valid {
			t.Errorf("case valid but generated error: %s", tc.name)
		}
		if (err == nil) && !tc.valid {
			t.Errorf("case invalid but did not generate error: %s", tc.name)
		}

	}
}
