package buyer

type BuyerStage int32
type reporterCtxKey int

const (
	// ReporterCtxKey is the key to use when passing a reporter via context
	ReporterCtxKey = reporterCtxKey(1)

	// minRequredConfirmations is the minimum number of confirmations the
	// inputs to the split ticket must have to be usable.
	minRequredConfirmations = 2
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
	StageSkippedWaiting
	StageWaitingPublishedTxs
	StageSessionEndedSuccessfully
)

const (
	// SplitTicketSrvService is the _service_ part of an SRV record that is
	// looked up prior to connecting to the split ticket service
	SplitTicketSrvService = "split-tickets-grpc"

	// SplitTicketSrvProto is the protocol to use when looking up an SRV
	// record for the split ticket service
	SplitTicketSrvProto = "tcp"
)
