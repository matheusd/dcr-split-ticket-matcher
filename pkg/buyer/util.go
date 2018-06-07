package buyer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
	pb "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/matcherrpc"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"
)

func encodeSessionName(name string) string {
	hash := sha256.Sum256([]byte(name))
	return hex.EncodeToString(hash[:])
}

// WriterReporter implements the BuyerReporter interface by generating descriptive
// messages to a writer
type WriterReporter struct {
	w io.Writer
}

// NewWriterReporter returns a reporter that writes into stdout
func NewWriterReporter(w io.Writer) *WriterReporter {
	return &WriterReporter{w}
}

// WaitingListChanged fulfills waitingListWatcher by outputting the changes
func (rep *WriterReporter) WaitingListChanged(queues []matcher.WaitingQueue) {
	for _, q := range queues {
		strs := make([]string, len(q.Amounts))
		sessName := q.Name
		if len(sessName) > 10 {
			sessName = sessName[:10]
		}
		for i, a := range q.Amounts {
			strs[i] = a.String()
		}
		fmt.Fprintf(rep.w, "Waiting participants (%s): [%s]\n", sessName, strings.Join(strs, ", "))
	}
}

func (rep *WriterReporter) reportSavedSession(fname string) {
	fmt.Fprintf(rep.w, "Saved session at %s\n", fname)
}

func (rep *WriterReporter) reportMatcherStatus(status *pb.StatusResponse) {
	price := dcrutil.Amount(status.TicketPrice)
	fmt.Fprintf(rep.w, "Matcher ticket price: %s\n", price)
}

func (rep *WriterReporter) reportSrvRecordFound(record string) {
	fmt.Fprintf(rep.w, "Found SRV record to use host %s\n", record)
}

func (rep *WriterReporter) reportSplitPublished() {
	fmt.Fprintf(rep.w, "Split tx published in the network\n")
}

func (rep *WriterReporter) reportRightTicketPublished() {
	fmt.Fprintf(rep.w, "Correct ticket published in the network\n")
}

func (rep *WriterReporter) reportWrongTicketPublished(ticket *wire.MsgTx, session *Session) {
	fmt.Fprintf(rep.w, "\n!!! WARNING !!!\n")
	fmt.Fprintf(rep.w, "Wrong ticket published in the network!\n")
	fmt.Fprintf(rep.w, "Please notify the community that this matcher is compromised IMMEDIATELY!!\n")
	fmt.Fprintf(rep.w, "Provide the saved session information for verification.\n")
	fmt.Fprintf(rep.w, "Published ticket hash: %s\n", ticket.TxHash())
	fmt.Fprintf(rep.w, "Expected ticket hash: %s\n", session.selectedTicket.TxHash())
}

func (rep *WriterReporter) reportStage(ctx context.Context, stage Stage, session *Session, cfg *Config) {

	out := func(format string, args ...interface{}) {
		fmt.Fprintf(rep.w, format, args...)
	}

	// initial handling of stages with null session
	switch stage {
	case StageUnknown:
		out("ERROR: Unknown stage received\n")
		return
	case StageConnectingToMatcher:
		out("Connecting to matcher service '%s'\n", cfg.MatcherHost)
		return
	case StageFindingMatches:
		encSessName := encodeSessionName(cfg.SessionName)[:10]
		out("Finding peers to split ticket buy in session '%s' (%s)\n",
			cfg.SessionName, encSessName)
		return
	case StageConnectingToWallet:
		out("Connecting to wallet %s\n", cfg.WalletHost)
		return
	}

	// from here on, all stages need a session
	if session == nil {
		out("Received null session on stage %d\n", stage)
		return
	}

	switch stage {
	case StageMatchesFound:
		out("Found matches in session %s. Contributing %s\n", session.ID, session.Amount)
	case StageGeneratingOutputs:
		out("Generating outputs from wallet\n")
	case StageOutputsGenerated:
		out("Outputs generated!\n")
		out("Voting address: %s\n", cfg.VoteAddress)
		out("Pool subsidy address: %s\n", cfg.PoolAddress)
		out("Ticket commitment address: %s\n", session.ticketOutputAddress.String())
		out("Split tx output address: %s\n", session.splitOutputAddress.String())
	case StageGeneratingTicket:
		out("Generating ticket...\n")
	case StageTicketGenerated:
		out("Ticket Generated\n")
		out("Secret Number: %d\n", session.secretNb)
		out("Secret Number Hash: %s\n", hex.EncodeToString(session.secretNbHash[:]))
		out("My index in the split: %d\n", session.myIndex)
	case StageGenerateSplitOutputAddr:
		out("Generating split output address\n")
	case StageGenerateTicketCommitmentAddr:
		out("Generating ticket commitment address\n")
	case StageGenerateSplitInputs:
		out("Generating split tx funds\n")
	case StageSigningTicket:
		out("Signing Ticket\n")
	case StageTicketSigned:
		out("Ticket Signed\n")
	case StageSigningRevocation:
		out("Signing Revocation\n")
	case StageRevocationSigned:
		out("Revocation Signed\n")
	case StageFundingTicket:
		out("Funding Ticket on matcher\n")
	case StageTicketFunded:
		out("Ticket Funded!\n")
	case StageSigningSplitTx:
		out("Signing split tx\n")
	case StageSplitTxSigned:
		out("Split tx signed\n")
	case StageFundingSplitTx:
		out("Funding split tx\n")
	case StageSplitTxFunded:
		out("Split tx funded!\n")
	case StageSkippedWaiting:
		out("Not waiting for published transactions.\n")
		out("Raw transaction data:\n")
		btsSplit, _ := session.fundedSplitTx.Bytes()
		out("\nFunded Split Tx:\n")
		out(hex.EncodeToString(btsSplit) + "\n")

		btsTicket, _ := session.selectedTicket.Bytes()
		out("\nFunded Ticket:\n")
		out(hex.EncodeToString(btsTicket) + "\n")

		btsRevoke, _ := session.selectedRevocation.Bytes()
		out("\nFunded Revocation:\n")
		out(hex.EncodeToString(btsRevoke) + "\n")
	case StageWaitingPublishedTxs:
		out("Waiting for txs to be published by matcher\n")
	case StageSessionEndedSuccessfully:
		out("Session ended successfully.\n")

		btsSplit, _ := session.fundedSplitTx.Bytes()
		btsTicket, _ := session.selectedTicket.Bytes()
		btsRevoke, _ := session.selectedRevocation.Bytes()

		splitFee, err := splitticket.FindTxFee(session.fundedSplitTx, session.splitTxUtxoMap)
		if err != nil {
			out("ERROR calculating split tx fee: %v", err)
		}

		ticketFee, err := splitticket.FindTicketTxFee(session.fundedSplitTx, session.selectedTicket)
		if err != nil {
			out("ERROR calculating ticket tx fee: %v", err)
		}

		revokeFee, err := splitticket.FindRevocationTxFee(session.selectedTicket, session.selectedRevocation)
		if err != nil {
			out("ERROR calculating revocation tx fee: %v", err)
		}

		splitFeeRate := float64(splitFee) / float64(len(btsSplit)*1e5)
		ticketFeeRate := float64(ticketFee) / float64(len(btsTicket)*1e5)
		revokeFeeRate := float64(revokeFee) / float64(len(btsRevoke)*1e5)

		out("Split tx size: %d bytes\n", len(btsSplit))
		out("Split tx fee: %s (%.4f DCR/KB)\n", splitFee, splitFeeRate)
		out("Ticket tx size: %d bytes\n", len(btsTicket))
		out("Ticket tx fee: %s (%.4f DCR/KB)\n", ticketFee, ticketFeeRate)
		out("Revoke tx size: %d bytes\n", len(btsRevoke))
		out("Revoke tx fee: %s (%.4f DCR/KB)\n", revokeFee, revokeFeeRate)

		out("\n")
		out("Selected coin: %s\n", session.selectedCoin)
		out("Selected voter index: %d\n", session.voterIndex)
		var sum dcrutil.Amount
		for i, p := range session.participants {
			sum += p.amount
			out("Participant %d: cum_amount=%s secret=%d secret_hash=%s...\n",
				i, sum, p.secretNb, hex.EncodeToString(p.secretHash[:10]))
		}

		commitHash := splitticket.CalcLotteryCommitmentHash(
			session.secretHashes(), session.amounts(), session.voteAddresses(),
			session.mainchainHash)

		out("Voter lottery commitment hash: %s\n",
			hex.EncodeToString(commitHash[:]))

		out("\n")
		out("Split tx hash: %s\n", session.fundedSplitTx.TxHash())
		out("Ticket Hash: %s\n", session.selectedTicket.TxHash())

	default:
		out("Unknown stage: %d\n", stage)
	}

}

// NullReporter is a dummy reporter that never outputs anything.
type NullReporter struct{}

func (rep NullReporter) reportStage(ctx context.Context, stage Stage, session *Session, cfg *Config) {
}
func (rep NullReporter) reportMatcherStatus(status *pb.StatusResponse)                   {}
func (rep NullReporter) reportSavedSession(string)                                       {}
func (rep NullReporter) reportSrvRecordFound(record string)                              {}
func (rep NullReporter) reportSplitPublished()                                           {}
func (rep NullReporter) reportRightTicketPublished()                                     {}
func (rep NullReporter) reportWrongTicketPublished(ticket *wire.MsgTx, session *Session) {}

func reporterFromContext(ctx context.Context) Reporter {
	val := ctx.Value(ReporterCtxKey)
	if rep, is := val.(Reporter); is {
		return rep
	}

	return NullReporter{}
}

func uint64sToAmounts(in []uint64) []dcrutil.Amount {
	res := make([]dcrutil.Amount, len(in))
	for i, a := range in {
		res[i] = dcrutil.Amount(a)
	}
	return res
}
