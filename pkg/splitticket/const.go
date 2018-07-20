package splitticket

const (
	// SplitTicketSrvService is the _service_ part of an SRV record that is
	// looked up prior to connecting to the split ticket service
	SplitTicketSrvService = "split-tickets-grpc"

	// SplitTicketSrvProto is the protocol to use when looking up an SRV
	// record for the split ticket service
	SplitTicketSrvProto = "tcp"
)
