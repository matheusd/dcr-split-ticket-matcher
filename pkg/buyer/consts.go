package buyer

import (
	"github.com/ansel1/merry"
)

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

var (
	ErrNoReporterSpecified                  = merry.New("Stage reporter not specified in context")
	ErrNotEnoughFundsFundSplit              = merry.New("Not enough funds to fund split transaction to required amount")
	ErrNoInputSignedOnTicket                = merry.New("No input was found signed on ticket")
	ErrWrongNumOfAddressesInVoteOut         = merry.New("Wrong number of addresses in vote output")
	ErrNoInputSignedOnRevocation            = merry.New("Input was not found signed on revocation")
	ErrWrongInputSignedOnSplit              = merry.New("Wrong input signed on split transaction")
	ErrMissingSigOnSplitTx                  = merry.New("Missing signature of desired split tx input")
	ErrSplitChangeOutputNotFoundOnConstruct = merry.New("Could not find the split change output when constructing the split transaction inputs")
	ErrWalletOnWrongNetwork                 = merry.New("Wallet on wrong network")
)
