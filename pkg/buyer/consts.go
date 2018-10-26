package buyer

// Stage represents a single stage of the full ticket buying process.
type Stage int32
type reporterCtxKey int

const (
	// ReporterCtxKey is the key to use when passing a reporter via context
	ReporterCtxKey = reporterCtxKey(1)

	// minRequredConfirmations is the minimum number of confirmations the
	// inputs to the split ticket must have to be usable.
	minRequredConfirmations = 2
)

// Following are the various stages the buyer can be in. They may not
// necessarily pass through all of these stages.
const (
	StageUnknown Stage = iota
	StageStarting
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
