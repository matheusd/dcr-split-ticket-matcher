package splitticket

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
)
