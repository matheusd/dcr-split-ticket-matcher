package buyer

import (
	"context"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
)

type BuyerConfig struct {
	WalletCertFile string
	WalletHost     string
	MatcherHost    string
	MaxAmount      dcrutil.Amount
	SourceAccount  uint32
	SStxFeeLimits  uint16
	ChainParams    *chaincfg.Params
	VoteAddress    string
	PoolAddress    string
	Passphrase     []byte
}

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
	wc, err := ConenctToWallet(cfg.WalletHost, cfg.WalletCertFile)
	if err != nil {
		return err
	}
	defer wc.Close()

	rep.reportStage(ctx, StageFindingMatches, nil, cfg)
	session, err := mc.Participate(ctx, cfg.MaxAmount)
	if err != nil {
		return err
	}

	rep.reportStage(ctx, StageMatchesFound, session, cfg)

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

	return nil
}
