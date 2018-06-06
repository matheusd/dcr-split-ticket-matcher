package buyer

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
	pbm "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/matcherrpc"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/buyer/internal/net"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"
	"github.com/pkg/errors"
)

type buyerSessionParticipant struct {
	secretHash   splitticket.SecretNumberHash
	secretNb     splitticket.SecretNumber
	votePkScript []byte
	poolPkScript []byte
	amount       dcrutil.Amount
	ticket       *wire.MsgTx
	revocation   *wire.MsgTx
	voteAddress  dcrutil.Address
}

type BuyerSession struct {
	ID          matcher.ParticipantID
	Amount      dcrutil.Amount
	Fee         dcrutil.Amount
	PoolFee     dcrutil.Amount
	TicketPrice dcrutil.Amount

	mainchainHash   *chainhash.Hash
	mainchainHeight uint32
	nbParticipants  uint32
	secretNb        splitticket.SecretNumber
	secretNbHash    splitticket.SecretNumberHash

	voteAddress         dcrutil.Address
	poolAddress         dcrutil.Address
	splitOutputAddress  dcrutil.Address
	ticketOutputAddress dcrutil.Address
	votePkScript        []byte
	poolPkScript        []byte
	splitChange         *wire.TxOut
	splitInputs         []*wire.TxIn
	participants        []buyerSessionParticipant
	splitTxUtxoMap      splitticket.UtxoMap
	myIndex             uint32

	ticketTemplate *wire.MsgTx
	splitTx        *wire.MsgTx
	revocation     *wire.MsgTx

	ticketsScriptSig    [][]byte // one for each participant
	revocationScriptSig []byte

	selectedTicket     *wire.MsgTx
	fundedSplitTx      *wire.MsgTx
	selectedRevocation *wire.MsgTx
	voterIndex         int
	selectedCoin       dcrutil.Amount
}

func (session *BuyerSession) secretHashes() []splitticket.SecretNumberHash {
	res := make([]splitticket.SecretNumberHash, len(session.participants))
	for i, p := range session.participants {
		res[i] = p.secretHash
	}
	return res
}

func (session *BuyerSession) secretNumbers() []splitticket.SecretNumber {
	res := make([]splitticket.SecretNumber, len(session.participants))
	for i, p := range session.participants {
		res[i] = p.secretNb
	}
	return res
}

func (session *BuyerSession) amounts() []dcrutil.Amount {
	res := make([]dcrutil.Amount, len(session.participants))
	for i, p := range session.participants {
		res[i] = p.amount
	}
	return res
}

func (session *BuyerSession) voteScripts() [][]byte {
	res := make([][]byte, len(session.participants))
	for i, p := range session.participants {
		res[i] = p.votePkScript
	}
	return res
}

func (session *BuyerSession) voteAddresses() []dcrutil.Address {
	res := make([]dcrutil.Address, len(session.participants))
	for i, p := range session.participants {
		res[i] = p.voteAddress
	}
	return res
}

func (session *BuyerSession) splitInputOutpoints() []wire.OutPoint {
	res := make([]wire.OutPoint, len(session.splitTx.TxIn))
	for i, in := range session.splitTx.TxIn {
		res[i] = in.PreviousOutPoint
	}
	return res
}

type Reporter interface {
	reportStage(context.Context, BuyerStage, *BuyerSession, *BuyerConfig)
	reportMatcherStatus(*pbm.StatusResponse)
	reportSavedSession(string)
	reportSrvRecordFound(record string)
	reportSplitPublished()
	reportRightTicketPublished()
	reportWrongTicketPublished(ticket *wire.MsgTx, session *BuyerSession)
}

type sessionWaiterResponse struct {
	mc      *MatcherClient
	wc      *WalletClient
	session *BuyerSession
	err     error
}

func BuySplitTicket(ctx context.Context, cfg *BuyerConfig) error {

	if cfg.WalletHost == "127.0.0.1:0" {
		hosts, err := net.FindListeningWallets(cfg.WalletCertFile, cfg.ChainParams)
		if err != nil {
			return errors.Wrapf(err, "error finding running wallet")
		}

		if len(hosts) != 1 {
			return errors.Errorf("found different number of running wallets "+
				"(%d) than expected", len(hosts))
		}

		cfg.WalletHost = hosts[0]
	}

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

	rep.reportStage(ctx, StageConnectingToWallet, nil, cfg)
	wc, err := ConnectToWallet(cfg.WalletHost, cfg.WalletCertFile)
	if err != nil {
		return sessionWaiterResponse{nil, nil, nil, err}
	}

	err = wc.CheckNetwork(ctx, cfg.ChainParams)
	if err != nil {
		return sessionWaiterResponse{nil, nil, nil, err}
	}

	err = wc.testVoteAddress(ctx, cfg)
	if err != nil {
		return sessionWaiterResponse{nil, nil, nil, err}
	}

	err = wc.testPassphrase(ctx, cfg)
	if err != nil {
		return sessionWaiterResponse{nil, nil, nil, err}
	}

	err = wc.testFunds(ctx, cfg)
	if err != nil {
		return sessionWaiterResponse{nil, nil, nil, err}
	}

	rep.reportStage(ctx, StageConnectingToMatcher, nil, cfg)
	mc, err := ConnectToMatcherService(ctx, cfg.MatcherHost, cfg.MatcherCertFile,
		cfg.networkCfg())
	if err != nil {
		return sessionWaiterResponse{nil, nil, nil, errors.Wrapf(err, "error connecting to matcher")}
	}

	status, err := mc.Status(ctx)
	if err != nil {
		return sessionWaiterResponse{nil, nil, nil, errors.Wrapf(err, "error getting status from matcher")}
	}
	rep.reportMatcherStatus(status)

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

	chainInfo, err := wc.currentChainInfo(ctx)
	if err != nil {
		return sessionWaiterResponse{nil, nil, nil, err}
	}
	if !chainInfo.bestBlockHash.IsEqual(session.mainchainHash) {
		return sessionWaiterResponse{nil, nil, nil, errors.Errorf("mainchain tip "+
			"of wallet (%s) not the same as matcher (%s)",
			chainInfo.bestBlockHash, session.mainchainHash)}
	}
	if chainInfo.bestBlockHeight != session.mainchainHeight {
		return sessionWaiterResponse{nil, nil, nil, errors.Errorf("mainchain height "+
			"of wallet (%d) not the same as matcher (%d)",
			chainInfo.bestBlockHeight, session.mainchainHeight)}
	}
	if chainInfo.ticketPrice != session.TicketPrice {
		return sessionWaiterResponse{nil, nil, nil, errors.Errorf("ticket price"+
			"of wallet (%s) not the same as matcher (%s)",
			chainInfo.ticketPrice, session.TicketPrice)}
	}

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

	rep.reportStage(ctx, StageSigningTicket, session, cfg)
	err = wc.SignTransactions(ctx, session, cfg)
	if err != nil {
		return err
	}
	rep.reportStage(ctx, StageTicketSigned, session, cfg)

	rep.reportStage(ctx, StageFundingTicket, session, cfg)
	err = mc.FundTicket(ctx, session, cfg)
	if err != nil {
		return err
	}
	rep.reportStage(ctx, StageTicketFunded, session, cfg)

	mc.network.monitorSession(ctx, session)

	rep.reportStage(ctx, StageFundingSplitTx, session, cfg)
	err = mc.FundSplitTx(ctx, session, cfg)
	if err != nil {
		return err
	}
	rep.reportStage(ctx, StageSplitTxFunded, session, cfg)

	err = saveSession(ctx, session, cfg)
	if err != nil {
		return errors.Wrapf(err, "error saving session")
	}

	if cfg.SkipWaitPublishedTxs {
		rep.reportStage(ctx, StageSkippedWaiting, session, cfg)
		rep.reportStage(ctx, StageSessionEndedSuccessfully, session, cfg)
		return nil
	}

	err = waitForPublishedTxs(ctx, session, cfg, mc.network)
	if err != nil {
		return errors.Wrapf(err, "error waiting for txs to be published")
	}

	rep.reportStage(ctx, StageSessionEndedSuccessfully, session, cfg)
	return nil
}

func waitForPublishedTxs(ctx context.Context, session *BuyerSession,
	cfg *BuyerConfig, net *decredNetwork) error {

	var notifiedSplit, notifiedTicket bool
	rep := reporterFromContext(ctx)

	expectedTicketHash := session.selectedTicket.TxHash()
	correctTicket := false

	for !notifiedSplit && !notifiedTicket {
		select {
		case <-ctx.Done():
			ctxErr := ctx.Err()
			if ctxErr != nil {
				return errors.Wrapf(ctxErr, "context error while waiting for"+
					"published txs")
			} else {
				return errors.Errorf("context done while waiting for published" +
					"txs")
			}
		default:
			if !notifiedSplit && net.publishedSplit {
				rep.reportSplitPublished()
				notifiedSplit = true
			}

			if !notifiedTicket && net.publishedTicket != nil {
				publishedHash := net.publishedTicket.TxHash()
				if expectedTicketHash.IsEqual(&publishedHash) {
					rep.reportRightTicketPublished()
					correctTicket = true
				} else {
					rep.reportWrongTicketPublished(net.publishedTicket, session)
				}
				notifiedTicket = true
			}

			time.Sleep(time.Millisecond * 250)
		}
	}

	if !correctTicket {
		return errors.Errorf("wrong ticket published to the network")
	}

	return nil
}

func saveSession(ctx context.Context, session *BuyerSession, cfg *BuyerConfig) error {

	rep := reporterFromContext(ctx)

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

	ticketHashHex := session.selectedTicket.TxHash().String()
	ticketBytes, err := session.selectedTicket.Bytes()
	if err != nil {
		return err
	}

	fname := filepath.Join(sessionDir, ticketHashHex)

	fflags := os.O_TRUNC | os.O_CREATE | os.O_WRONLY
	f, err := os.OpenFile(fname, fflags, 0600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	hexWriter := hex.NewEncoder(w)

	defer func() {
		w.Flush()
		f.Sync()
		f.Close()
	}()

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

	out := func(format string, args ...interface{}) {
		w.WriteString(fmt.Sprintf(format, args...))
	}

	out("====== General Info ======\n")

	out("Session ID = %s\n", session.ID)
	out("Ending Time = %s\n", time.Now().String())
	out("Mainchain Hash = %s\n", session.mainchainHash.String())
	out("Mainchain Height = %d\n", session.mainchainHeight)
	out("Ticket Price = %s\n", session.TicketPrice)
	out("Number of Participants = %d\n", session.nbParticipants)
	out("My Index = %d\n", session.myIndex)
	out("My Secret Number = %d\n", session.secretNb)
	out("My Secret Hash = %s\n", session.secretNbHash)
	out("Commitment Amount = %s\n", session.Amount)
	out("Ticket Fee = %s (total = %s)\n", session.Fee, session.Fee*dcrutil.Amount(session.nbParticipants))
	out("Pool Fee = %s (total = %s)\n", session.PoolFee, session.PoolFee*dcrutil.Amount(session.nbParticipants))
	out("Split Transaction hash = %s\n", splitHash.String())
	out("Final Ticket Hash = %s\n", ticketHashHex)
	out("Final Revocation Hash = %s\n", revocationHash.String())

	out("\n")
	out("====== Voter Selection ======\n")

	commitHash := splitticket.CalcLotteryCommitmentHash(
		session.secretHashes(), session.amounts(), session.voteAddresses(),
		session.mainchainHash)

	out("Participant Amounts = %v\n", session.amounts())
	out("Secret Hashes = %v\n", session.secretHashes())
	out("Voter Lottery Commitment Hash = %s\n", hex.EncodeToString(commitHash[:]))
	out("Secret Numbers = %v\n", session.secretNumbers())
	out("Selected Coin = %s\n", session.selectedCoin)
	out("Selected Voter Index = %d\n", session.voterIndex)

	out("\n")
	out("====== Final Transactions ======\n")

	out("== Split Transaction ==\n")
	hexWriter.Write(splitBytes)
	out("\n\n")

	out("== Ticket ==\n")
	hexWriter.Write(ticketBytes)
	out("\n\n")

	out("== Revocation ==\n")
	hexWriter.Write(revocationBytes)
	out("\n\n")

	out("\n")
	out("====== My Split Inputs ======\n")
	for i, in := range session.splitInputs {
		out("Outpoint %d = %s\n", i, in.PreviousOutPoint)
	}

	out("\n")
	out("====== Participant Intermediate Information ======\n")
	for i, p := range session.participants {
		voteScript := hex.EncodeToString(p.votePkScript)
		poolScript := hex.EncodeToString(p.poolPkScript)

		partTicket, err := p.ticket.Bytes()
		if err != nil {
			return errors.Wrapf(err, "error encoding participant %d ticket", i)
		}

		partRevocation, err := p.revocation.Bytes()
		if err != nil {
			return errors.Wrapf(err, "error encoding participant %d revocation", i)
		}

		out("\n")
		out("== Participant %d ==\n", i)
		out("Amount = %s\n", p.amount)
		out("Secret Hash = %s\n", p.secretHash)
		out("Secret Number = %d\n", p.secretNb)
		out("Vote PkScript = %s\n", voteScript)
		out("Pool PkScript = %s\n", poolScript)
		out("Ticket = %s\n", hex.EncodeToString(partTicket))
		out("Revocation = %s\n", hex.EncodeToString(partRevocation))
	}

	w.Flush()
	f.Sync()

	rep.reportSavedSession(fname)

	return nil
}
