package buyer

import (
	"bytes"
	"context"
	"math/rand"
	"time"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"
	"github.com/pkg/errors"

	pb "github.com/decred/dcrwallet/rpc/walletrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func random(min, max int) int {
	return rand.Intn(max-min) + min
}

type WalletClient struct {
	conn *grpc.ClientConn
	wsvc pb.WalletServiceClient
}

func ConnectToWallet(walletHost string, walletCert string) (*WalletClient, error) {
	rand.Seed(time.Now().Unix())
	creds, err := credentials.NewClientTLSFromFile(walletCert, "localhost")
	if err != nil {
		return nil, err
	}

	conn, err := grpc.Dial(walletHost, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, err
	}

	wsvc := pb.NewWalletServiceClient(conn)

	wc := &WalletClient{
		conn: conn,
		wsvc: wsvc,
	}

	return wc, nil
}

func (wc *WalletClient) Close() error {
	return wc.conn.Close()
}

func (wc *WalletClient) CheckNetwork(ctx context.Context, chainParams *chaincfg.Params) error {
	req := &pb.NetworkRequest{}
	resp, err := wc.wsvc.Network(ctx, req)
	if err != nil {
		return err
	}

	if resp.ActiveNetwork != uint32(chainParams.Net) {
		return errors.Errorf("wallet (%d) not running on the expected "+
			"network (%d)", resp.ActiveNetwork, chainParams.Net)
	}

	return nil
}

func (wc *WalletClient) GenerateOutputs(ctx context.Context, session *BuyerSession, cfg *BuyerConfig) error {
	splitOutAddr, splitChangeAddr, ticketOutAdd, err := wc.generateOutputAddresses(ctx, session, cfg)
	session.ticketOutputAddress = ticketOutAdd
	session.splitOutputAddress = splitOutAddr

	splitChangeScript, err := txscript.PayToAddrScript(splitChangeAddr)
	if err != nil {
		return err
	}
	session.splitChange = wire.NewTxOut(0, splitChangeScript)

	return wc.generateSplitTxInputs(ctx, session, cfg)
}

func (wc *WalletClient) generateOutputAddresses(ctx context.Context, session *BuyerSession, cfg *BuyerConfig) (
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
		return
	}
	splitOut, err = dcrutil.DecodeAddress(resp.Address)
	if err != nil {
		return
	}

	// vvvvvvvv ticket output vvvvvvvv
	rep.reportStage(ctx, StageGenerateTicketCommitmentAddr, session, cfg)
	resp, err = wc.wsvc.NextAddress(ctx, req)
	if err != nil {
		return
	}
	ticketOut, err = dcrutil.DecodeAddress(resp.Address)
	if err != nil {
		return
	}

	// vvvvv split change vvvv
	req.Kind = pb.NextAddressRequest_BIP0044_INTERNAL
	resp, err = wc.wsvc.NextAddress(ctx, req)
	if err != nil {
		return
	}
	splitChange, err = dcrutil.DecodeAddress(resp.Address)
	if err != nil {
		return
	}

	return
}

func (wc *WalletClient) generateSplitTxInputs(ctx context.Context, session *BuyerSession, cfg *BuyerConfig) error {

	rep := reporterFromContext(ctx)

	outputs := make([]*pb.ConstructTransactionRequest_Output, 3)

	// add the output for the voter lottery commitment, so the wallet accounts
	// for that when calculating the amount of input.
	// FIXME: currently double paying, as all participants are accounting for
	// this. This should be of size ceil(33/nbParts)
	nullData := bytes.Repeat([]byte{0x00}, splitticket.SecretNumberHashesHashSize)
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
		FeePerKb:              0, // whatever is the default wallet fee
		RequiredConfirmations: 1,
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

	foundChangeOut := false
	for _, out := range tx.TxOut {
		if bytes.Equal(out.PkScript, splitChangeDest.Script) {
			foundChangeOut = true
			session.splitChange.Value = out.Value
		}
	}
	if !foundChangeOut {
		return errors.Errorf("split change not found on contructed split tx")
	}

	session.splitInputs = make([]*wire.TxIn, len(tx.TxIn))
	for i, in := range tx.TxIn {
		outp := wire.NewOutPoint(&in.PreviousOutPoint.Hash, in.PreviousOutPoint.Index, in.PreviousOutPoint.Tree)
		session.splitInputs[i] = wire.NewTxIn(outp, nil)
	}

	return nil
}

func (wc *WalletClient) prepareTicketsForSigning(session *BuyerSession) (
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
		bytes, err := ticket.Bytes()
		if err != nil {
			return nil, nil, err
		}

		tickets[i] = &pb.SignTransactionsRequest_UnsignedTransaction{
			SerializedTransaction: bytes,
		}
	}

	return tickets, splitScripts, nil
}

func (wc *WalletClient) processSignedTickets(
	transactions []*pb.SignTransactionsResponse_SignedTransaction,
	session *BuyerSession) error {

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

func (wc *WalletClient) prepareRevocationForSigning(session *BuyerSession) (
	*pb.SignTransactionsRequest_UnsignedTransaction,
	[]*pb.SignTransactionsRequest_AdditionalScript, error) {

	// create the ticket assuming I'm the one voting, then create a revocation
	// based on it, then sign it.
	myPart := session.participants[session.myIndex]
	myTicket := session.ticketTemplate.Copy()
	myTicket.TxOut[0].PkScript = myPart.votePkScript
	myTicket.TxOut[1].PkScript = myPart.poolPkScript

	ticketHash := myTicket.TxHash()

	revocation, err := matcher.CreateUnsignedRevocation(&ticketHash, myTicket,
		dcrutil.Amount(matcher.RevocationFeeRate))
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
	session *BuyerSession) error {

	signed := wire.NewMsgTx()
	signed.FromBytes(transaction.Transaction)

	if signed.TxIn[0].SignatureScript == nil {
		return errors.Errorf("input 0 was not signed on revocation")
	}

	session.revocationScriptSig = signed.TxIn[0].SignatureScript

	return nil
}

func (wc *WalletClient) splitPkScripts(splitCopy *wire.MsgTx,
	session *BuyerSession) ([]*pb.SignTransactionsRequest_AdditionalScript,
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

func (wc *WalletClient) prepareSplitForSigning(session *BuyerSession) (
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
	session *BuyerSession) error {

	signed := wire.NewMsgTx()
	signed.FromBytes(transaction.Transaction)

	// ensure my wallet is only signing the explicit inputs I previously sent
	// (to avoid someone injecting an input known to be mine)
	err := splitticket.CheckOnlySignedInSplit(signed, session.splitInputOutpoints())
	if err != nil {
		return err
	}

	signedCount := 0
	for _, in := range signed.TxIn {
		if in.SignatureScript == nil {
			continue
		}

		for _, inSplit := range session.splitInputs {
			if (bytes.Equal(inSplit.PreviousOutPoint.Hash[:], in.PreviousOutPoint.Hash[:])) &&
				(inSplit.PreviousOutPoint.Index == in.PreviousOutPoint.Index) {

				inSplit.SignatureScript = in.SignatureScript
				signedCount++
			}
			// } else {
			// 	return ErrWrongInputSignedOnSplit
			// }
		}
	}

	if signedCount != len(session.splitInputs) {
		return errors.Errorf("number of signed inputs of split tx (%d) "+
			"different than expected (%d)", signedCount,
			len(session.splitInputs))
	}

	return nil
}

func (wc *WalletClient) SignTransactions(ctx context.Context, session *BuyerSession, cfg *BuyerConfig) error {
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

	// FIXME: remove on production... doing this in many wallets at the same time
	// locks them up
	time.Sleep(time.Millisecond * time.Duration(random(100, 2000)))

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
