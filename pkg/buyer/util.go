package buyer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	pb "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/matcherrpc"
)

func encodeSessionName(name string) string {
	hash := sha256.Sum256([]byte(name))
	return hex.EncodeToString(hash[:])
}

// StdOutReporter implements the BuyerReporter interface by generating descriptive
// messages to stdout
type StdOutReporter struct {
}

func (rep *StdOutReporter) reportMatcherStatus(status *pb.StatusResponse) {
	price := dcrutil.Amount(status.TicketPrice)
	fmt.Printf("Matcher ticket price: %s\n", price)
}

func (rep *StdOutReporter) reportStage(ctx context.Context, stage BuyerStage, session *BuyerSession, cfg *BuyerConfig) {

	// initial handling of stages with null session
	switch stage {
	case StageUnknown:
		fmt.Printf("ERROR: Unknown stage received\n")
		return
	case StageConnectingToMatcher:
		fmt.Printf("Connecting to matcher service '%s'\n", cfg.MatcherHost)
		return
	case StageFindingMatches:
		encSessName := encodeSessionName(cfg.SessionName)[:10]
		fmt.Printf("Finding peers to split ticket buy in session '%s' (%s)\n",
			cfg.SessionName, encSessName)
		return
	case StageConnectingToWallet:
		fmt.Printf("Connecting to wallet %s\n", cfg.WalletHost)
		return
	}

	// from here on, all stages need a session
	if session == nil {
		fmt.Printf("Received null session on stage %d\n", stage)
		return
	}

	switch stage {
	case StageMatchesFound:
		fmt.Printf("Found matches in session %s. Contributing %s\n", session.ID, session.Amount)
	case StageGeneratingOutputs:
		fmt.Printf("Generating outputs from wallet\n")
	case StageOutputsGenerated:
		fmt.Printf("Outputs generated!\n")
		fmt.Printf("Voting address: %s\n", cfg.VoteAddress)
		fmt.Printf("Pool subsidy address: %s\n", cfg.PoolAddress)
		fmt.Printf("Ticket commitment address: %s\n", session.ticketOutputAddress.String())
		fmt.Printf("Split tx output address: %s\n", session.splitOutputAddress.String())
	case StageGeneratingTicket:
		fmt.Printf("Generating ticket...\n")
	case StageTicketGenerated:
		fmt.Printf("Ticket Generated\n")
		fmt.Printf("Secret Number: %d\n", session.secretNb)
		fmt.Printf("Secret Number Hash: %s\n", hex.EncodeToString(session.secretNbHash[:]))
		fmt.Printf("My index in the split: %d\n", session.myIndex)
	case StageGenerateSplitOutputAddr:
		fmt.Printf("Generating split output address\n")
	case StageGenerateTicketCommitmentAddr:
		fmt.Printf("Generating ticket commitment address\n")
	case StageGenerateSplitInputs:
		fmt.Printf("Generating split tx funds\n")
	case StageSigningTicket:
		fmt.Printf("Signing Ticket\n")
	case StageTicketSigned:
		fmt.Printf("Ticket Signed\n")
	case StageSigningRevocation:
		fmt.Printf("Signing Revocation\n")
	case StageRevocationSigned:
		fmt.Printf("Revocation Signed\n")
	case StageFundingTicket:
		fmt.Printf("Funding Ticket on matcher\n")
	case StageTicketFunded:
		fmt.Printf("Ticket Funded!\n")
	case StageSigningSplitTx:
		fmt.Printf("Signing split tx\n")
	case StageSplitTxSigned:
		fmt.Printf("Split tx signed\n")
	case StageFundingSplitTx:
		fmt.Printf("Funding split tx\n")
	case StageSplitTxFunded:
		fmt.Printf("Split tx funded\n")

		bts, _ := session.fundedSplitTx.Bytes()
		fmt.Println("\nFunded Split Tx:")
		fmt.Println(hex.EncodeToString(bts))

		bts, _ = session.selectedTicket.Bytes()
		fmt.Println("\nFunded Ticket:")
		fmt.Println(hex.EncodeToString(bts))

		bts, _ = session.selectedRevocation.Bytes()
		fmt.Println("\nFunded Revocation:")
		fmt.Println(hex.EncodeToString(bts))

		fmt.Println("")
		fmt.Printf("Selected coin: %s\n", dcrutil.Amount(session.selectedCoin()))
		fmt.Printf("Selected voter index: %d\n", session.voterIndex)
		var sum dcrutil.Amount
		for i, p := range session.participants {
			sum += p.amount
			fmt.Printf("Participant %d: cum_amount=%s secret=%d secret_hash=%s...\n",
				i, sum, p.secretNb, hex.EncodeToString(p.secretHash[:10]))
		}
		commitHash := matcher.SecretNumberHashesHash(session.secretHashes(),
			session.mainchainHash)
		fmt.Printf("Voter lottery commitment hash: %s\n",
			hex.EncodeToString(commitHash))

	default:
		fmt.Printf("Unknown stage: %d\n", stage)
	}

}

type NullReporter struct{}

func (rep NullReporter) reportStage(ctx context.Context, stage BuyerStage, session *BuyerSession, cfg *BuyerConfig) {
}

func (rep NullReporter) reportMatcherStatus(status *pb.StatusResponse) {
}

func reporterFromContext(ctx context.Context) Reporter {
	val := ctx.Value(ReporterCtxKey)
	if rep, is := val.(Reporter); is {
		return rep
	}

	return NullReporter{}
}

func dummyScriptSigner(net *chaincfg.Params) (pkScript []byte, scriptSig []byte) {
	var err error

	script := []byte{txscript.OP_NOP}

	scriptAddr, err := dcrutil.NewAddressScriptHash(script, net)
	if err != nil {
		panic(err)
	}

	pkScript, err = txscript.PayToAddrScript(scriptAddr)
	if err != nil {
		panic(err)
	}

	b := txscript.NewScriptBuilder()
	b.AddOp(txscript.OP_TRUE)
	b.AddData(script)

	scriptSig, err = b.Script()
	if err != nil {
		panic(err)
	}

	return
}

func uint64sToAmounts(in []uint64) []dcrutil.Amount {
	res := make([]dcrutil.Amount, len(in))
	for i, a := range in {
		res[i] = dcrutil.Amount(a)
	}
	return res
}
