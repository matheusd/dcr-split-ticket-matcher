package buyer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	pb "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/matcherrpc"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/internal/util"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/version"
)

func encodeSessionName(name string) string {
	hash := sha256.Sum256([]byte(name))
	return hex.EncodeToString(hash[:])
}

// LoggerMiddleware allows to log both to a file and standard output
type LoggerMiddleware struct {
	w       io.Writer
	logFile *os.File
}

// NewLoggerMiddleware returns a new middleware to write to both a log file and
// the standard log reporing output
func NewLoggerMiddleware(w io.Writer, logDir string) *LoggerMiddleware {
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		os.MkdirAll(logDir, 0755)
	}

	fname := util.LogFileName(logDir, "splitticketbuyer-{date}-{time}.log")
	f, err := os.OpenFile(fname, os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		panic(err)
	}

	return &LoggerMiddleware{
		w:       w,
		logFile: f,
	}
}

// Write fulfills io.Writer
func (lm *LoggerMiddleware) Write(p []byte) (n int, err error) {
	fmt.Fprintf(lm.logFile, "%s: ", time.Now().Format("2006-01-02 15:04:05.000"))
	lm.logFile.Write(p)
	lm.logFile.Sync()

	return lm.w.Write(p)
}

// WriterReporter implements the BuyerReporter interface by generating descriptive
// messages to a writer
type WriterReporter struct {
	w                io.Writer
	sessionName      string
	lastWaitListLine string
}

// NewWriterReporter returns a reporter that writes into stdout
func NewWriterReporter(w io.Writer, sessionName string) *WriterReporter {
	sessionHash := sha256.Sum256([]byte(sessionName))
	sessionName = hex.EncodeToString(sessionHash[:])
	return &WriterReporter{
		w:           w,
		sessionName: sessionName,
	}
}

// WaitingListChanged fulfills waitingListWatcher by outputting the changes
func (rep *WriterReporter) WaitingListChanged(queues []matcher.WaitingQueue) {
	for _, q := range queues {
		if q.Name != rep.sessionName {
			continue
		}
		strs := make([]string, len(q.Amounts))
		total := dcrutil.Amount(0)
		for i, a := range q.Amounts {
			strs[i] = a.String()
			total += a
		}
		line := strings.Join(strs, ", ")
		if line != rep.lastWaitListLine {
			fmt.Fprintf(rep.w, "Waiting participants (%s): [%s]\n", total, line)
			rep.lastWaitListLine = line
		}
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

func (rep *WriterReporter) reportSrvLookupError(err error) {
	fmt.Fprintf(rep.w, "SRV lookup error: %s\n", err)
}

func (rep *WriterReporter) reportSplitPublished() {
	fmt.Fprintf(rep.w, "Split tx published in the network\n")
}

func (rep *WriterReporter) reportRightTicketPublished() {
	fmt.Fprintf(rep.w, "Correct ticket published in the network\n")
}

func (rep *WriterReporter) reportWrongTicketPublished(ticket *chainhash.Hash, session *Session) {
	fmt.Fprintf(rep.w, "\n!!! WARNING !!!\n")
	fmt.Fprintf(rep.w, "Wrong ticket published in the network!\n")
	fmt.Fprintf(rep.w, "Please notify the community that this matcher is compromised IMMEDIATELY!!\n")
	fmt.Fprintf(rep.w, "Provide the saved session information for verification.\n")
	fmt.Fprintf(rep.w, "Published ticket hash: %s\n", ticket.String())
	fmt.Fprintf(rep.w, "Expected ticket hash: %s\n", session.selectedTicket.TxHash())
}

func (rep *WriterReporter) reportBuyingError(err error) {
	fmt.Fprintf(rep.w, "Error buying split ticket: %s\n", err.Error())
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
	case StageStarting:
		out("Starting purchase process v%s\n", version.String())
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
	case StageConnectingToDcrd:
		out("Connecting to dcrd %s\n", cfg.DcrdHost)
		return
	case StageConnectingToDcrdata:
		out("Verified dcrdata online %s\n", cfg.DcrdataURL)
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
		out("Secret Number: %s\n", session.secretNb)
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
			out("Participant %d: cum_amount=%s secret=%s secret_hash=%s...\n",
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
func (rep NullReporter) reportMatcherStatus(status *pb.StatusResponse)                       {}
func (rep NullReporter) reportSavedSession(string)                                           {}
func (rep NullReporter) reportSrvRecordFound(record string)                                  {}
func (rep NullReporter) reportSrvLookupError(err error)                                      {}
func (rep NullReporter) reportSplitPublished()                                               {}
func (rep NullReporter) reportRightTicketPublished()                                         {}
func (rep NullReporter) reportWrongTicketPublished(ticket *chainhash.Hash, session *Session) {}
func (rep NullReporter) reportBuyingError(err error)                                         {}

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

func encodedVoteAddresses(addrs []dcrutil.Address) []string {
	res := make([]string, len(addrs))
	for i, addr := range addrs {
		res[i] = addr.EncodeAddress()
	}
	return res
}
