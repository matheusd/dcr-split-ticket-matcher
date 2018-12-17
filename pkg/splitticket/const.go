package splitticket

import "github.com/decred/dcrd/dcrutil"

const (
	// SplitTicketSrvService is the _service_ part of an SRV record that is
	// looked up prior to connecting to the split ticket service
	SplitTicketSrvService = "split-tickets-grpc"

	// SplitTicketSrvProto is the protocol to use when looking up an SRV
	// record for the split ticket service
	SplitTicketSrvProto = "tcp"

	// MinimumSplitInputConfirms is how many confirmations any utxo being used
	// in the split transaction must have before being allowed in
	MinimumSplitInputConfirms = 2

	// MaximumSplitInputs is the maximum number of inputs into a split
	// transaction that a single participant should provide.
	//
	// Sending too many inputs might cause a DoS in either the server or the
	// participants, so servers should limit participants to this number of
	// inputs. Participants need to consolidate their funds if they have more
	// utxos than this.
	//
	// As of 2018-11-09 only ~3% of all mainnet txs used more than 20 inputs, so
	// this should be reasonable.
	MaximumSplitInputs = 20

	// TxFeeRate is the expected transaction fee rate for the split and ticket
	// transactions of participants of split tickets.
	//
	// Measured as Atoms/KB. 1e5 = 0.001 DCR
	TxFeeRate dcrutil.Amount = 1e5
)
