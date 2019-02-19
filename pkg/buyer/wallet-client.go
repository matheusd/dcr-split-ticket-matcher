package buyer

import (
	"bytes"
	"context"
	"math/rand"
	"time"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"
	"github.com/pkg/errors"

	pb "github.com/decred/dcrwallet/rpc/walletrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

// WalletClient is responsible for the interactions of the buyer with the local
// wallet daemon (dcrwallet).
type WalletClient struct {
	conn            *grpc.ClientConn
	wsvc            pb.WalletServiceClient
	chainParams     *chaincfg.Params
	publishedSplit  bool
	publishedTicket *wire.MsgTx
}

type currentChainInfo struct {
	bestBlockHash   *chainhash.Hash
	bestBlockHeight uint32
	ticketPrice     dcrutil.Amount
}

// ConnectToWallet connects to the given wallet address
func ConnectToWallet(walletHost string, walletCert string, chainParams *chaincfg.Params) (*WalletClient, error) {
	rand.Seed(time.Now().Unix())
	creds, err := credentials.NewClientTLSFromFile(walletCert, "localhost")
	if err != nil {
		return nil, err
	}

	optCreds := grpc.WithTransportCredentials(creds)

	connCtx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	// wallet doesn't allow config of keepalive for the moment, so disable it
	// and perform a manual ping.
	// optKeepAlive := grpc.WithKeepaliveParams(keepalive.ClientParameters{
	// 	Time:                30 * time.Second,
	// 	Timeout:             5 * time.Second,
	// 	PermitWithoutStream: true,
	// })

	conn, err := grpc.DialContext(connCtx, walletHost, optCreds)
	if err != nil {
		return nil, err
	}

	wsvc := pb.NewWalletServiceClient(conn)

	wc := &WalletClient{
		conn:        conn,
		wsvc:        wsvc,
		chainParams: chainParams,
	}

	return wc, nil
}

func (wc *WalletClient) close() error {
	return wc.conn.Close()
}

// checkWalletWaitingForSession repeatedly pings wallet connection while the
// buyer is waiting for a session, so that if the wallet is closed the buyer is
// alerted about this fact.
// If context is canceled, then this returns nil.
// This blocks, therefore it MUST be run from a goroutine.
func (wc *WalletClient) checkWalletWaitingForSession(waitCtx context.Context) error {
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			return nil
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			_, err := wc.wsvc.Ping(ctx, &pb.PingRequest{})
			cancel()
			if err != nil {
				return errors.Wrap(err, "error trying to check wallet connection")
			}
		}
	}
}

func (wc *WalletClient) checkNetwork(ctx context.Context) error {
	req := &pb.NetworkRequest{}
	resp, err := wc.wsvc.Network(ctx, req)
	if err != nil {
		return err
	}

	if resp.ActiveNetwork != uint32(wc.chainParams.Net) {
		return errors.Errorf("wallet (%d) not running on the expected "+
			"network (%d)", resp.ActiveNetwork, wc.chainParams.Net)
	}

	return nil
}

func (wc *WalletClient) generateOutputs(ctx context.Context, session *Session, cfg *Config) error {
	splitOutAddr, splitChangeAddr, ticketOutAdd, err := wc.generateOutputAddresses(ctx, session, cfg)
	if err != nil {
		return errors.Wrapf(err, "error generating output addresses")
	}

	session.ticketOutputAddress = ticketOutAdd
	session.splitOutputAddress = splitOutAddr

	splitChangeScript, err := txscript.PayToAddrScript(splitChangeAddr)
	if err != nil {
		return errors.Wrapf(err, "error creating pay2addr script")
	}
	session.splitChange = wire.NewTxOut(0, splitChangeScript)

	return wc.generateSplitTxInputs(ctx, session, cfg)
}

func (wc *WalletClient) generateOutputAddresses(ctx context.Context, session *Session, cfg *Config) (
	splitOut, splitChange, ticketOut dcrutil.Address, err error) {

	var req *pb.NextAddressRequest
	var resp *pb.NextAddressResponse
	rep := reporterFromContext(ctx)

	req = &pb.NextAddressRequest{
		Account:   cfg.SourceAccount,
		GapPolicy: pb.NextAddressRequest_GAP_POLICY_WRAP,
		Kind:      pb.NextAddressRequest_BIP0044_EXTERNAL,
	}

	// vvvvv split output vvvvv
	rep.reportStage(ctx, StageGenerateSplitOutputAddr, session, cfg)
	resp, err = wc.wsvc.NextAddress(ctx, req)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "error obtaining next address "+
			"for split output")
	}
	splitOut, err = dcrutil.DecodeAddress(resp.Address)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "error decoding address for "+
			"split output")
	}

	// vvvvvvvv ticket output vvvvvvvv
	rep.reportStage(ctx, StageGenerateTicketCommitmentAddr, session, cfg)
	resp, err = wc.wsvc.NextAddress(ctx, req)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "error obtaining next address "+
			"for ticket commitment output")
	}
	ticketOut, err = dcrutil.DecodeAddress(resp.Address)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "error decoding address for "+
			"ticket commitment output")
	}

	// vvvvv split change vvvv
	req.Kind = pb.NextAddressRequest_BIP0044_INTERNAL
	resp, err = wc.wsvc.NextAddress(ctx, req)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "error obtaining next address "+
			"for split change output")
	}
	splitChange, err = dcrutil.DecodeAddress(resp.Address)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "error decoding address for "+
			"split change output")
	}

	return
}

func (wc *WalletClient) generateSplitTxInputs(ctx context.Context, session *Session, cfg *Config) error {

	rep := reporterFromContext(ctx)

	outputs := make([]*pb.ConstructTransactionRequest_Output, 3)

	// add the output for the voter lottery commitment, so the wallet accounts
	// for that when calculating the amount of input.
	// FIXME: currently double paying, as all participants are accounting for
	// this. This should be of size ceil(33/nbParts)
	nullData := bytes.Repeat([]byte{0x00}, splitticket.LotteryCommitmentHashSize)
	script := append([]byte{txscript.OP_RETURN, txscript.OP_DATA_32}, nullData...)
	outputs[0] = &pb.ConstructTransactionRequest_Output{
		Amount: 0,
		Destination: &pb.ConstructTransactionRequest_OutputDestination{
			Script: script,
		},
	}

	// add the output for my ticket commitment
	outputs[1] = &pb.ConstructTransactionRequest_Output{
		Amount: int64(session.Amount + session.Fee),
		Destination: &pb.ConstructTransactionRequest_OutputDestination{
			Address: session.splitOutputAddress.String(),
		},
	}

	// add a dummy pool fee output, so that we account for our share of the pool fee
	// when grabbing funds for the split tx
	zeroed := [20]byte{}
	addrZeroed, err := dcrutil.NewAddressPubKeyHash(zeroed[:], cfg.ChainParams, 0)
	if err != nil {
		return err
	}
	outputs[2] = &pb.ConstructTransactionRequest_Output{
		Amount: int64(session.PoolFee),
		Destination: &pb.ConstructTransactionRequest_OutputDestination{
			Address: addrZeroed.String(),
		},
	}

	splitChangeDest := &pb.ConstructTransactionRequest_OutputDestination{
		Script:        session.splitChange.PkScript,
		ScriptVersion: uint32(session.splitChange.Version),
	}

	req := &pb.ConstructTransactionRequest{
		FeePerKb:              int32(splitticket.TxFeeRate),
		RequiredConfirmations: splitticket.MinimumSplitInputConfirms,
		SourceAccount:         cfg.SourceAccount,
		NonChangeOutputs:      outputs,
		ChangeDestination:     splitChangeDest,
	}

	rep.reportStage(ctx, StageGenerateSplitInputs, session, cfg)
	resp, err := wc.wsvc.ConstructTransaction(ctx, req)
	if err != nil {
		return err
	}

	tx := wire.NewMsgTx()
	err = tx.FromBytes(resp.UnsignedTransaction)
	if err != nil {
		return err
	}

	if resp.ChangeIndex > -1 {
		out := tx.TxOut[resp.ChangeIndex]
		if !bytes.Equal(out.PkScript, splitChangeDest.Script) {
			return errors.Errorf("wallet changed split change pkscript")
		}

		session.splitChange.Value = out.Value
	} else {
		session.splitChange = nil
	}

	session.splitInputs = make([]*wire.TxIn, len(tx.TxIn))
	for i, in := range tx.TxIn {
		outp := wire.NewOutPoint(&in.PreviousOutPoint.Hash, in.PreviousOutPoint.Index, in.PreviousOutPoint.Tree)
		session.splitInputs[i] = wire.NewTxIn(outp, wire.NullValueIn, nil)
	}

	return nil
}

func (wc *WalletClient) prepareTicketsForSigning(session *Session) (
	[]*pb.SignTransactionsRequest_UnsignedTransaction,
	[]*pb.SignTransactionsRequest_AdditionalScript, error) {

	splitTxHash := session.splitTx.TxHash()

	splitScripts := make([]*pb.SignTransactionsRequest_AdditionalScript, len(session.splitTx.TxOut))
	for i, out := range session.splitTx.TxOut {
		splitScripts[i] = &pb.SignTransactionsRequest_AdditionalScript{
			OutputIndex:     uint32(i),
			PkScript:        out.PkScript,
			TransactionHash: splitTxHash[:],
			Tree:            int32(wire.TxTreeRegular),
		}
	}

	ticket := session.ticketTemplate.Copy()

	tickets := make([]*pb.SignTransactionsRequest_UnsignedTransaction, len(session.participants))
	for i, p := range session.participants {
		ticket.TxOut[0].PkScript = p.votePkScript
		ticket.TxOut[1].PkScript = p.poolPkScript
		bts, err := ticket.Bytes()
		if err != nil {
			return nil, nil, err
		}

		tickets[i] = &pb.SignTransactionsRequest_UnsignedTransaction{
			SerializedTransaction: bts,
		}
	}

	return tickets, splitScripts, nil
}

func (wc *WalletClient) processSignedTickets(
	transactions []*pb.SignTransactionsResponse_SignedTransaction,
	session *Session) error {

	signed := wire.NewMsgTx()

	for _, t := range transactions {

		err := signed.FromBytes(t.Transaction)
		if err != nil {
			return err
		}

		for i, in := range signed.TxIn {
			// FIXME not really great to get the signed input by checking if (i > 0)
			// ideally we should get the ticket output index by the matcher
			if (in.SignatureScript != nil) && (len(in.SignatureScript) > 0) && (i > 0) {
				// TODO: instead of appending, size it once before the loop.
				session.ticketsScriptSig = append(session.ticketsScriptSig, in.SignatureScript)
				break
			}
		}
	}

	if len(session.ticketsScriptSig) != len(session.participants) {
		return errors.Errorf("no input was signed on the ticket")
	}

	return nil
}

func (wc *WalletClient) prepareRevocationForSigning(session *Session) (
	*pb.SignTransactionsRequest_UnsignedTransaction,
	[]*pb.SignTransactionsRequest_AdditionalScript, error) {

	// create the ticket assuming I'm the one voting, then create a revocation
	// based on it, then sign it.
	myPart := session.participants[session.myIndex]
	myTicket := session.ticketTemplate.Copy()
	myTicket.TxOut[0].PkScript = myPart.votePkScript
	myTicket.TxOut[1].PkScript = myPart.poolPkScript

	ticketHash := myTicket.TxHash()

	revocation, err := splitticket.CreateUnsignedRevocation(&ticketHash, myTicket,
		splitticket.RevocationFeeRate(wc.chainParams))
	if err != nil {
		return nil, nil, err
	}

	revocationBytes, err := revocation.Bytes()
	if err != nil {
		return nil, nil, err
	}

	ticketScripts := make([]*pb.SignTransactionsRequest_AdditionalScript, 1)
	ticketScripts[0] = &pb.SignTransactionsRequest_AdditionalScript{
		OutputIndex:     0,
		PkScript:        myTicket.TxOut[0].PkScript,
		TransactionHash: ticketHash[:],
		Tree:            int32(wire.TxTreeStake),
	}

	txReq := &pb.SignTransactionsRequest_UnsignedTransaction{
		SerializedTransaction: revocationBytes,
	}

	return txReq, ticketScripts, nil

}

func (wc *WalletClient) processSignedRevocation(
	transaction *pb.SignTransactionsResponse_SignedTransaction,
	session *Session) error {

	signed := wire.NewMsgTx()
	signed.FromBytes(transaction.Transaction)

	if signed.TxIn[0].SignatureScript == nil {
		return errors.Errorf("input 0 was not signed on revocation")
	}

	session.revocationScriptSig = signed.TxIn[0].SignatureScript

	return nil
}

func (wc *WalletClient) splitPkScripts(splitCopy *wire.MsgTx,
	session *Session) ([]*pb.SignTransactionsRequest_AdditionalScript,
	error) {

	scripts := make([]*pb.SignTransactionsRequest_AdditionalScript, len(splitCopy.TxIn))
	for i, in := range splitCopy.TxIn {
		utxo, hasUtxo := session.splitTxUtxoMap[in.PreviousOutPoint]
		if !hasUtxo {
			return nil, errors.Errorf("did not find utxo for input %d of "+
				"split tx", i)
		}

		scripts[i] = &pb.SignTransactionsRequest_AdditionalScript{
			TransactionHash: in.PreviousOutPoint.Hash[:],
			OutputIndex:     in.PreviousOutPoint.Index,
			Tree:            int32(in.PreviousOutPoint.Tree),
			PkScript:        utxo.PkScript,
		}
	}

	return scripts, nil

}

func (wc *WalletClient) prepareSplitForSigning(session *Session) (
	*pb.SignTransactionsRequest_UnsignedTransaction,
	[]*pb.SignTransactionsRequest_AdditionalScript, error) {

	splitCopy := session.splitTx.Copy()

	scripts, err := wc.splitPkScripts(splitCopy, session)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error grabing pkscript of split "+
			"tx inputs")
	}

	splitBytes, err := splitCopy.Bytes()
	if err != nil {
		return nil, nil, err
	}

	txReq := &pb.SignTransactionsRequest_UnsignedTransaction{
		SerializedTransaction: splitBytes,
	}

	return txReq, scripts, nil
}

func (wc *WalletClient) processSignedSplit(
	transaction *pb.SignTransactionsResponse_SignedTransaction,
	session *Session) error {

	signed := wire.NewMsgTx()
	signed.FromBytes(transaction.Transaction)

	// Ensure my wallet is only signing the explicit inputs I previously sent
	// (to avoid someone injecting an input known to be mine).
	err := splitticket.CheckOnlySignedInSplit(signed, session.mySplitInputOupoints())
	if err != nil {
		return err
	}

	// Make an aux map of the selected outpoints => *TxIn, so that we can
	// directly reference it in the next loop.
	splitInputMap := make(map[wire.OutPoint]*wire.TxIn, len(session.splitInputs))
	for _, in := range session.splitInputs {
		splitInputMap[in.PreviousOutPoint] = in
	}

	// Copy the signature scripts for the wallet-signed inputs.
	for _, in := range signed.TxIn {
		if len(in.SignatureScript) == 0 {
			continue
		}

		if inSplit, has := splitInputMap[in.PreviousOutPoint]; has {
			// This test should never actually fail due to the previous
			// CheckOnlySignedInSplit() call, but is left in case that function
			// ever changes, since it ensures only signatures for the previously
			// selected inputs are copied.
			inSplit.SignatureScript = in.SignatureScript
		}
	}

	return nil
}

func (wc *WalletClient) signTransactions(ctx context.Context, session *Session, cfg *Config) error {
	req := &pb.SignTransactionsRequest{
		Passphrase: cfg.Passphrase,
	}

	tickets, ticketsAddScripts, err := wc.prepareTicketsForSigning(session)
	if err != nil {
		return errors.Wrapf(err, "error preparing tickets for signing")
	}

	revocation, revocationAddScripts, err := wc.prepareRevocationForSigning(session)
	if err != nil {
		return errors.Wrapf(err, "error preparing revocation for signing")
	}

	split, splitAddScripts, err := wc.prepareSplitForSigning(session)
	if err != nil {
		return errors.Wrapf(err, "error preparing split tx for signing")
	}

	req.Transactions = append(req.Transactions, split, revocation)
	req.Transactions = append(req.Transactions, tickets...)
	req.AdditionalScripts = append(req.AdditionalScripts, splitAddScripts...)
	req.AdditionalScripts = append(req.AdditionalScripts, revocationAddScripts...)
	req.AdditionalScripts = append(req.AdditionalScripts, ticketsAddScripts...)

	resp, err := wc.wsvc.SignTransactions(ctx, req)
	if err != nil {
		return errors.Wrapf(err, "error signing transactions")
	}

	expectedLen := 1 + 1 + len(tickets)
	foundLen := len(resp.Transactions)
	if foundLen != expectedLen {
		return errors.Errorf("wallet signed a different number transactions "+
			"(%d) than expected (%d)", foundLen, expectedLen)
	}

	if err := wc.processSignedSplit(resp.Transactions[0], session); err != nil {
		return errors.Wrapf(err, "error processing signed split tx")
	}

	if err := wc.processSignedRevocation(resp.Transactions[1], session); err != nil {
		return errors.Wrapf(err, "error processing signed revocation tx")
	}

	if err := wc.processSignedTickets(resp.Transactions[2:], session); err != nil {
		return errors.Wrapf(err, "error processing signed tickets")
	}

	return nil

}

// testVoteAddress tests whether the vote address is signable by the local
// wallet
func (wc *WalletClient) testVoteAddress(ctx context.Context, cfg *Config) error {
	resp, err := wc.wsvc.ValidateAddress(context.Background(), &pb.ValidateAddressRequest{
		Address: cfg.VoteAddress,
	})
	if err != nil {
		return errors.Wrapf(err, "error determining validity of vote address")
	}

	if !resp.IsValid {
		return errors.Errorf("vote address is not valid")
	}

	if !resp.IsMine {
		return errors.Errorf("vote address is not from local wallet")
	}

	return nil
}

// testPassphrase tests whether the configured password is correct by attempting
// to sign a message. It generates an address on the internal branch of the
// wallet.
func (wc *WalletClient) testPassphrase(ctx context.Context, cfg *Config) error {

	resp, err := wc.wsvc.NextAddress(ctx, &pb.NextAddressRequest{
		Account:   cfg.SourceAccount,
		GapPolicy: pb.NextAddressRequest_GAP_POLICY_WRAP,
		Kind:      pb.NextAddressRequest_BIP0044_INTERNAL,
	})

	if err != nil {
		return errors.Wrapf(err, "error generating address to test passphrase")
	}

	req := &pb.SignMessageRequest{
		Address:    resp.Address,
		Message:    "Pretty please, sign this :P",
		Passphrase: cfg.Passphrase,
	}

	_, err = wc.wsvc.SignMessage(ctx, req)
	if err != nil {
		return errors.Wrapf(err, "error signing message to test passphrase")
	}

	return nil
}

// testFunds tests whether the wallet has sufficient funds to contribute the
// specified maxAmount into a split ticket session.
//
// It returns the maximum contribution amount when accounting for possible
func (wc *WalletClient) testFunds(ctx context.Context, cfg *Config) error {

	amount, err := dcrutil.NewAmount(cfg.MaxAmount)
	if err != nil {
		return errors.Wrapf(err, "error decoding maxAmount")
	}

	zeroed := [20]byte{}
	addrP2PKH, _ := dcrutil.NewAddressPubKeyHash(zeroed[:], cfg.ChainParams, 0)

	// estimate the worst case (largest contribution amount): a split ticket
	// where we are the sole participant.
	outputs := make([]*pb.ConstructTransactionRequest_Output, 4)

	// split the total amount in 3 parts, one for each simulated output and add
	// the ticket fee
	ticketFee := splitticket.SessionFeeEstimate(1)
	poolFee := amount / 3
	changeAmount := poolFee
	amount = amount - poolFee - changeAmount + ticketFee

	// participanting with the maximum contribution (assuming the amount is
	// enough for a full ticket)
	outputs[0] = &pb.ConstructTransactionRequest_Output{
		Amount: int64(amount),
		Destination: &pb.ConstructTransactionRequest_OutputDestination{
			Address: cfg.VoteAddress,
		},
	}

	// force the tx to have a simulated change output so that we ensure we
	// contribute with enough to generate change.
	outputs[1] = &pb.ConstructTransactionRequest_Output{
		Amount: int64(changeAmount),
		Destination: &pb.ConstructTransactionRequest_OutputDestination{
			Address: addrP2PKH.EncodeAddress(),
		},
	}

	// pool fee output
	outputs[2] = &pb.ConstructTransactionRequest_Output{
		Amount: int64(poolFee),
		Destination: &pb.ConstructTransactionRequest_OutputDestination{
			Address: cfg.PoolAddress,
		},
	}

	// lottery commitment output
	nullData2 := bytes.Repeat([]byte{0x00}, splitticket.LotteryCommitmentHashSize)
	script := append([]byte{txscript.OP_RETURN, txscript.OP_DATA_32}, nullData2...)
	outputs[3] = &pb.ConstructTransactionRequest_Output{
		Amount: 0,
		Destination: &pb.ConstructTransactionRequest_OutputDestination{
			Script: script,
		},
	}

	splitChangeDest := &pb.ConstructTransactionRequest_OutputDestination{
		Address:       addrP2PKH.EncodeAddress(),
		ScriptVersion: uint32(txscript.DefaultScriptVersion),
	}

	req := &pb.ConstructTransactionRequest{
		FeePerKb:              int32(splitticket.TxFeeRate),
		RequiredConfirmations: splitticket.MinimumSplitInputConfirms,
		SourceAccount:         cfg.SourceAccount,
		NonChangeOutputs:      outputs,
		ChangeDestination:     splitChangeDest,
	}

	resp, err := wc.wsvc.ConstructTransaction(ctx, req)
	errCode := status.Code(err)
	if errCode == codes.ResourceExhausted {
		// not enough funds to participate in split session. Let's see if the
		// issue is unconfirmed funds by changing requiredConfirmations to 0

		req.RequiredConfirmations = 0
		_, errConfirm := wc.wsvc.ConstructTransaction(ctx, req)
		errCodeConfirm := status.Code(errConfirm)
		if errCodeConfirm == codes.ResourceExhausted {
			// it's not. We really don't have enough funds, so error out
			// appropriately
			return errors.Errorf("Not enough funds to participate with given "+
				"maximum amount (tested for %s + split tx fees)",
				amount+poolFee+changeAmount)
		} else if errCodeConfirm == codes.OK {
			// it is. Then alert the user that it's a matter of having enough
			// confirmations.
			return errors.Errorf("Not enough confirmed funds to participate. " +
				"Wait a few more blocks for confirmation.")
		}
	}
	if err != nil {
		return errors.Wrapf(err, "error testing funds for session")
	}

	if resp.ChangeIndex < 0 {
		return errors.Wrapf(err, "no change back to wallet when testing funds "+
			"for transaction")
	}

	tx := new(wire.MsgTx)
	err = tx.FromBytes(resp.UnsignedTransaction)
	if err != nil {
		return errors.Wrapf(err, "error unserializing response tx")
	}
	if len(tx.TxIn) > splitticket.MaximumSplitInputs {
		return errors.Errorf("number of utxos to send into split tx (%d) "+
			"larger than maximum allowed (%d)", len(tx.TxIn),
			splitticket.MaximumSplitInputs)
	}

	return nil
}

func (wc *WalletClient) currentChainInfo(ctx context.Context) (*currentChainInfo, error) {
	resp, err := wc.wsvc.BestBlock(ctx, &pb.BestBlockRequest{})
	if err != nil {
		return nil, errors.Wrapf(err, "error getting best block from wallet")
	}

	respTicket, err := wc.wsvc.TicketPrice(ctx, &pb.TicketPriceRequest{})
	if err != nil {
		return nil, errors.Wrapf(err, "error getting current ticket price "+
			"from wallet")
	}

	hash, err := chainhash.NewHash(resp.Hash)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating hash from wallet")
	}

	return &currentChainInfo{
		bestBlockHash:   hash,
		bestBlockHeight: resp.Height,
		ticketPrice:     dcrutil.Amount(respTicket.TicketPrice),
	}, nil
}

// monitorSession monitors the given session (split tx and possible tickets) for
// publishing. Assumes the split and ticket templates have been received and the
// vote/pool pkscripts of individual participants have also been received.
//
// This starts a new goroutine that will watch over new wallet transactions,
// registering whether it has seen the split and ticket transactions.
func (wc *WalletClient) monitorSession(ctx context.Context, sess *Session) error {

	ticketsHashes := make(map[chainhash.Hash]*wire.MsgTx, sess.nbParticipants)
	for _, p := range sess.participants {
		ticketsHashes[p.ticket.TxHash()] = p.ticket
	}
	splitHash := sess.splitTx.TxHash()

	ntfs, err := wc.wsvc.TransactionNotifications(ctx, &pb.TransactionNotificationsRequest{})
	if err != nil {
		return errors.Wrap(err, "error attaching to transaction notifications")
	}
	go func() {
		for {
			resp, err := ntfs.Recv()
			if err != nil {
				// probably is due to the context closing
				return
			}

			for _, tx := range resp.UnminedTransactions {
				txh, err := chainhash.NewHash(tx.Hash)
				if err != nil {
					// maybe log?
					continue
				}

				if !wc.publishedSplit && splitHash.IsEqual(txh) {
					wc.publishedSplit = true
					continue
				}

				ticket, has := ticketsHashes[*txh]
				if has {
					wc.publishedTicket = ticket
				}
			}

			// unlikley to appear directly as a mined tx, but we should be thorough
			for _, block := range resp.GetAttachedBlocks() {
				for _, tx := range block.Transactions {
					txh, err := chainhash.NewHash(tx.Hash)
					if err != nil {
						// maybe log?
						continue
					}

					if !wc.publishedSplit && splitHash.IsEqual(txh) {
						wc.publishedSplit = true
						continue
					}

					ticket, has := ticketsHashes[*txh]
					if has {
						wc.publishedTicket = ticket
					}
				}
			}

			if wc.publishedSplit && (wc.publishedTicket != nil) {
				// got both transactions. can exit.
				return
			}
		}
	}()

	return nil
}
