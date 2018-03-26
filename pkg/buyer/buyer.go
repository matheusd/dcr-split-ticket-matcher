package buyer

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
)

type BuyerSession struct {
	ID      matcher.ParticipantID
	Amount  dcrutil.Amount
	Fee     dcrutil.Amount
	PoolFee dcrutil.Amount

	splitOutputAddress  dcrutil.Address
	ticketOutputAddress dcrutil.Address
	ticketOutput        *wire.TxOut
	ticketChange        *wire.TxOut
	splitOutput         *wire.TxOut
	splitChange         *wire.TxOut
	splitInputs         []*wire.TxIn

	ticket     *wire.MsgTx
	splitTx    *wire.MsgTx
	revocation *wire.MsgTx
	isVoter    bool

	ticketScriptSig     []byte
	revocationScriptSig []byte
}

type Reporter interface {
	reportStage(context.Context, BuyerStage, *BuyerSession, *BuyerConfig)
}

func BuySplitTicket(ctx context.Context, cfg *BuyerConfig) error {
	ctx, _ = context.WithTimeout(ctx, time.Second*time.Duration(cfg.MaxTime))
	reschan := make(chan error)
	go func() { reschan <- buySplitTicket(ctx, cfg) }()

	select {
	case <-ctx.Done():
		<-reschan // Wait for f to return.
		return ctx.Err()
	case err := <-reschan:
		return err
	}
}

func buySplitTicket(ctx context.Context, cfg *BuyerConfig) error {

	repVal := ctx.Value(ReporterCtxKey)
	if repVal == nil {
		return ErrNoReporterSpecified
	}
	rep, is := repVal.(Reporter)
	if !is {
		return ErrNoReporterSpecified
	}

	rep.reportStage(ctx, StageConnectingToMatcher, nil, cfg)
	mc, err := ConnectToMatcherService(cfg.MatcherHost)
	if err != nil {
		return err
	}
	defer mc.Close()

	rep.reportStage(ctx, StageConnectingToWallet, nil, cfg)
	wc, err := ConnectToWallet(cfg.WalletHost, cfg.WalletCertFile)
	if err != nil {
		return err
	}
	defer wc.Close()

	err = wc.CheckNetwork(ctx, cfg.ChainParams)
	if err != nil {
		return err
	}

	err = mc.WatchWaitingList(ctx)
	if err != nil {
		return err
	}
	time.Sleep(150 * time.Millisecond)

	maxAmount, err := dcrutil.NewAmount(cfg.MaxAmount)
	if err != nil {
		return err
	}

	rep.reportStage(ctx, StageFindingMatches, nil, cfg)
	session, err := mc.Participate(ctx, maxAmount)
	if err != nil {
		return err
	}

	rep.reportStage(ctx, StageMatchesFound, session, cfg)

	time.Sleep(5 * time.Second)

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
	err = wc.SignTicket(ctx, session, cfg)
	if err != nil {
		return err
	}
	rep.reportStage(ctx, StageTicketSigned, session, cfg)

	if session.isVoter {
		rep.reportStage(ctx, StageSigningRevocation, session, cfg)
		err = wc.SignRevocation(ctx, session, cfg)
		if err != nil {
			return err
		}
		rep.reportStage(ctx, StageRevocationSigned, session, cfg)
	}

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

	ticketHash := session.ticket.TxHash()
	ticketHashHex := hex.EncodeToString(ticketHash[:])
	ticketBytes, err := session.ticket.Bytes()
	if err != nil {
		return err
	}

	fname := filepath.Join(sessionDir, ticketHashHex)

	f, err := os.Create(fname)
	if err != nil {
		return err
	}
	defer f.Close()

	splitHash := session.splitTx.TxHash()
	splitBytes, err := session.splitTx.Bytes()
	if err != nil {
		return err
	}

	revocationHash := session.revocation.TxHash()
	revocationBytes, err := session.revocation.Bytes()
	if err != nil {
		return err
	}

	w := bufio.NewWriter(f)
	hexWriter := hex.NewEncoder(w)
	w.WriteString("Split ticket session ")
	w.WriteString(time.Now().String())
	w.WriteString(fmt.Sprintf("\nIsVoter = %t\n", session.isVoter))
	w.WriteString(fmt.Sprintf("Amount = %s\n", session.Amount))
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

	return nil
}
