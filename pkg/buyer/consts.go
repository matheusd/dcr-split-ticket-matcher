package buyer

type BuyerStage int32
type reporterCtxKey int

const (
	ReporterCtxKey = reporterCtxKey(1)
)

const (
	StageUnknown BuyerStage = iota
	StageConnectingToMatcher
	StageConnectingToWallet
	StageFindingMatches
	StageMatchesFound
	StageGeneratingOutputs
	StageGenerateSplitOutputAddr
	StageGenerateTicketCommitmentAddr
	StageGenerateSplitInputs
	StageOutputsGenerated
	StageGeneratingTicket
	StageTicketGenerated
	StageSigningTicket
	StageTicketSigned
	StageSigningRevocation
	StageRevocationSigned
	StageFundingTicket
	StageTicketFunded
	StageSigningSplitTx
	StageSplitTxSigned
	StageFundingSplitTx
	StageSplitTxFunded
)

const (
	// SplitTicketSrvService is the _service_ part of an SRV record that is
	// looked up prior to connecting to the split ticket service
	SplitTicketSrvService = "split-tickets-grpc"

	// SplitTicketSrvProto is the protocol to use when looking up an SRV
	// record for the split ticket service
	SplitTicketSrvProto = "tcp"
)
