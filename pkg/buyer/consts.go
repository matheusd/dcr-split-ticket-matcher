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
