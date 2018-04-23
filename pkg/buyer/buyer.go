package buyer

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
	pbm "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/matcherrpc"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
)

type buyerSessionParticipant struct {
	secretHash   matcher.SecretNumberHash
	secretNb     matcher.SecretNumber
	votePkScript []byte
	poolPkScript []byte
	amount       dcrutil.Amount
	ticket       *wire.MsgTx
	revocation   *wire.MsgTx
}

type BuyerSession struct {
	ID           matcher.ParticipantID
	Amount       dcrutil.Amount
	Fee          dcrutil.Amount
	PoolFee      dcrutil.Amount
	TotalPoolFee dcrutil.Amount

	mainchainHash *chainhash.Hash
	secretNb      matcher.SecretNumber
	secretNbHash  matcher.SecretNumberHash

	splitOutputAddress  dcrutil.Address
	ticketOutputAddress dcrutil.Address
	votePkScript        []byte
	poolPkScript        []byte
	ticketOutput        *wire.TxOut
	ticketChange        *wire.TxOut
	splitOutput         *wire.TxOut
	splitChange         *wire.TxOut
	splitInputs         []*wire.TxIn
	participants        []buyerSessionParticipant

	ticketTemplate *wire.MsgTx
	splitTx        *wire.MsgTx
	revocation     *wire.MsgTx

	ticketsScriptSig    [][]byte // one for each participant
	revocationScriptSig []byte

	selectedTicket     *wire.MsgTx
	fundedSplitTx      *wire.MsgTx
	selectedRevocation *wire.MsgTx
	voterIndex         int
}

func (session *BuyerSession) selectedCoin() uint64 {
	var totalCommitment uint64
	nbs := make(matcher.SecretNumbers, len(session.participants))
	for i, p := range session.participants {
		totalCommitment += uint64(p.amount)
		nbs[i] = p.secretNb
	}

	nbsHash := nbs.Hash(*session.mainchainHash)
	return nbsHash.SelectedCoin(totalCommitment)
}

func (session *BuyerSession) findVoterIndex() int {
	var sum uint64
	coinIdx := session.selectedCoin()
	for i, p := range session.participants {
		sum += uint64(p.amount)
		if coinIdx < sum {
			return i
		}
	}

	return -1
}

type Reporter interface {
	reportStage(context.Context, BuyerStage, *BuyerSession, *BuyerConfig)
	reportMatcherStatus(*pbm.StatusResponse)
}

type stdoutListWatcher struct{}

func (w stdoutListWatcher) ListChanged(queues []matcher.WaitingQueue) {
	for _, q := range queues {
		strs := make([]string, len(q.Amounts))
		sessName := q.Name
		if len(sessName) > 10 {
			sessName = sessName[:10]
		}
		for i, a := range q.Amounts {
			strs[i] = a.String()
		}
		fmt.Printf("Waiting participants (%s): [%s]\n", sessName, strings.Join(strs, ", "))
	}
}

type sessionWaiterResponse struct {
	mc      *MatcherClient
	wc      *WalletClient
	session *BuyerSession
	err     error
}

func BuySplitTicket(ctx context.Context, cfg *BuyerConfig) error {
	ctxWait, _ := context.WithTimeout(ctx, time.Second*time.Duration(cfg.MaxWaitTime))
	var resp sessionWaiterResponse
	reschan := make(chan sessionWaiterResponse)
	go func() { reschan <- waitForSession(ctxWait, cfg) }()

	select {
	case <-ctx.Done():
		<-reschan // Wait for f to return.
		return ctx.Err()
	case resp = <-reschan:
		if resp.err != nil {
			return resp.err
		}
	}

	defer func() {
		resp.mc.Close()
		resp.wc.Close()
	}()

	ctxBuy, _ := context.WithTimeout(ctx, time.Second*time.Duration(cfg.MaxTime))
	reschan2 := make(chan error)
	go func() { reschan2 <- buySplitTicket(ctxBuy, cfg, resp.mc, resp.wc, resp.session) }()

	select {
	case <-ctx.Done():
		<-reschan2 // Wait for f to return.
		return ctx.Err()
	case err := <-reschan2:
		return err
	}

}

func waitForSession(ctx context.Context, cfg *BuyerConfig) sessionWaiterResponse {
	rep := reporterFromContext(ctx)

	rep.reportStage(ctx, StageConnectingToMatcher, nil, cfg)
	mc, err := ConnectToMatcherService(cfg.MatcherHost, cfg.MatcherCertFile)
	if err != nil {
		return sessionWaiterResponse{nil, nil, nil, err}
	}

	status, err := mc.Status(ctx)
	if err != nil {
		return sessionWaiterResponse{nil, nil, nil, err}
	}
	rep.reportMatcherStatus(status)

	rep.reportStage(ctx, StageConnectingToWallet, nil, cfg)
	wc, err := ConnectToWallet(cfg.WalletHost, cfg.WalletCertFile)
	if err != nil {
		return sessionWaiterResponse{nil, nil, nil, err}
	}

	err = wc.CheckNetwork(ctx, cfg.ChainParams)
	if err != nil {
		return sessionWaiterResponse{nil, nil, nil, err}
	}

	err = mc.WatchWaitingList(ctx, stdoutListWatcher{})
	if err != nil {
		return sessionWaiterResponse{nil, nil, nil, err}
	}
	time.Sleep(150 * time.Millisecond)

	maxAmount, err := dcrutil.NewAmount(cfg.MaxAmount)
	if err != nil {
		return sessionWaiterResponse{nil, nil, nil, err}
	}

	rep.reportStage(ctx, StageFindingMatches, nil, cfg)
	session, err := mc.Participate(ctx, maxAmount, cfg.SessionName)
	if err != nil {
		return sessionWaiterResponse{nil, nil, nil, err}
	}
	rep.reportStage(ctx, StageMatchesFound, session, cfg)

	return sessionWaiterResponse{mc, wc, session, nil}
}

func buySplitTicket(ctx context.Context, cfg *BuyerConfig, mc *MatcherClient, wc *WalletClient, session *BuyerSession) error {

	rep := reporterFromContext(ctx)
	var err error

	rep.reportStage(ctx, StageGeneratingOutputs, session, cfg)
	err = wc.GenerateOutputs(ctx, session, cfg)
	if err != nil {
		return err
	}
	rep.reportStage(ctx, StageOutputsGenerated, session, cfg)

	rep.reportStage(ctx, StageGeneratingTicket, session, cfg)
	err = mc.GenerateTicket(ctx, session, cfg)
	if err != nil {
		return err
	}
	rep.reportStage(ctx, StageTicketGenerated, session, cfg)

	// FIXME: make the client-side checks to ensure the ticket is valid

	rep.reportStage(ctx, StageSigningTicket, session, cfg)
	err = wc.SignTickets(ctx, session, cfg)
	if err != nil {
		return err
	}
	rep.reportStage(ctx, StageTicketSigned, session, cfg)

	rep.reportStage(ctx, StageSigningRevocation, session, cfg)
	err = wc.SignRevocation(ctx, session, cfg)
	if err != nil {
		return err
	}
	rep.reportStage(ctx, StageRevocationSigned, session, cfg)

	rep.reportStage(ctx, StageFundingTicket, session, cfg)
	err = mc.FundTicket(ctx, session, cfg)
	if err != nil {
		return err
	}
	rep.reportStage(ctx, StageTicketFunded, session, cfg)

	rep.reportStage(ctx, StageSigningSplitTx, session, cfg)
	err = wc.SignSplit(ctx, session, cfg)
	if err != nil {
		return err
	}
	rep.reportStage(ctx, StageSplitTxSigned, session, cfg)

	rep.reportStage(ctx, StageFundingSplitTx, session, cfg)
	err = mc.FundSplitTx(ctx, session, cfg)
	if err != nil {
		return err
	}
	rep.reportStage(ctx, StageSplitTxFunded, session, cfg)

	return saveSession(session, cfg)
}

func saveSession(session *BuyerSession, cfg *BuyerConfig) error {

	sessionDir := filepath.Join(cfg.DataDir, "sessions")

	_, err := os.Stat(sessionDir)

	if os.IsNotExist(err) {
		err := os.MkdirAll(sessionDir, 0700)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	ticketHash := session.selectedTicket.TxHash()
	ticketHashHex := hex.EncodeToString(ticketHash[:])
	ticketBytes, err := session.selectedTicket.Bytes()
	if err != nil {
		return err
	}

	fname := filepath.Join(sessionDir, ticketHashHex)

	f, err := os.Create(fname)
	if err != nil {
		return err
	}
	defer f.Close()

	splitHash := session.fundedSplitTx.TxHash()
	splitBytes, err := session.fundedSplitTx.Bytes()
	if err != nil {
		return err
	}

	revocationHash := session.selectedRevocation.TxHash()
	revocationBytes, err := session.selectedRevocation.Bytes()
	if err != nil {
		return err
	}

	w := bufio.NewWriter(f)
	hexWriter := hex.NewEncoder(w)
	w.WriteString("Split ticket session ")
	w.WriteString(time.Now().String())
	w.WriteString(fmt.Sprintf("Amount = %s\n", session.Amount))
	w.WriteString(fmt.Sprintf("Session ID = %s\n", session.ID))
	w.WriteString("\n")

	w.WriteString("Split Transaction hash: ")
	hexWriter.Write(splitHash[:])
	w.WriteString("\nSplit Transaction:\n")
	hexWriter.Write(splitBytes)
	w.WriteString("\n\n")

	w.WriteString("Ticket hash: ")
	hexWriter.Write(ticketHash[:])
	w.WriteString("\nTicket:\n")
	hexWriter.Write(ticketBytes)
	w.WriteString("\n\n")

	w.WriteString("Revocation hash: ")
	hexWriter.Write(revocationHash[:])
	w.WriteString("\nRevocation:\n")
	hexWriter.Write(revocationBytes)
	w.WriteString("\n\n")

	w.Flush()
	f.Sync()

	fmt.Printf("\n\nSaved session at %s\n", fname)

	return nil
}
