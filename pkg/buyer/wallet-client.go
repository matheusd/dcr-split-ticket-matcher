package buyer

import (
	"bytes"
	"context"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"

	pb "github.com/decred/dcrwallet/rpc/walletrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type WalletClient struct {
	conn *grpc.ClientConn
	wsvc pb.WalletServiceClient
}

func ConnectToWallet(walletHost string, walletCert string) (*WalletClient, error) {
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
		return ErrWalletOnWrongNetwork
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

	outputs := make([]*pb.ConstructTransactionRequest_Output, 2)
	outputs[0] = &pb.ConstructTransactionRequest_Output{
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
	outputs[1] = &pb.ConstructTransactionRequest_Output{
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
		return ErrSplitChangeOutputNotFoundOnConstruct
	}

	session.splitInputs = make([]*wire.TxIn, len(tx.TxIn))
	for i, in := range tx.TxIn {
		outp := wire.NewOutPoint(&in.PreviousOutPoint.Hash, in.PreviousOutPoint.Index, in.PreviousOutPoint.Tree)
		session.splitInputs[i] = wire.NewTxIn(outp, nil)
	}

	return nil
}

func (wc *WalletClient) SignTickets(ctx context.Context, session *BuyerSession, cfg *BuyerConfig) error {

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
			return err
		}

		tickets[i] = &pb.SignTransactionsRequest_UnsignedTransaction{
			SerializedTransaction: bytes,
		}
	}

	req := &pb.SignTransactionsRequest{
		Passphrase:        cfg.Passphrase,
		AdditionalScripts: splitScripts,
		Transactions:      tickets,
	}

	resp, err := wc.wsvc.SignTransactions(ctx, req)
	if err != nil {
		return err
	}

	signed := wire.NewMsgTx()

	for _, t := range resp.Transactions {

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
		return ErrNoInputSignedOnTicket
	}

	return nil
}

func (wc *WalletClient) SignRevocation(ctx context.Context, session *BuyerSession, cfg *BuyerConfig) error {

	// create the ticket assuming I'm the one voting, then create a revocation
	// based on it, then sign it.
	myPart := session.participants[session.myIndex]
	myTicket := session.ticketTemplate.Copy()
	myTicket.TxOut[0].PkScript = myPart.votePkScript
	myTicket.TxOut[1].PkScript = myPart.poolPkScript

	ticketHash := myTicket.TxHash()

	revocationFee, _ := dcrutil.NewAmount(0.001) // TODO: parametrize
	revocation, err := matcher.CreateUnsignedRevocation(&ticketHash, myTicket,
		revocationFee)
	if err != nil {
		return err
	}

	revocationBytes, err := revocation.Bytes()
	if err != nil {
		return err
	}

	ticketScripts := make([]*pb.SignTransactionRequest_AdditionalScript, 1)
	ticketScripts[0] = &pb.SignTransactionRequest_AdditionalScript{
		OutputIndex:     0,
		PkScript:        myTicket.TxOut[0].PkScript,
		TransactionHash: ticketHash[:],
		Tree:            int32(wire.TxTreeStake),
	}

	req := &pb.SignTransactionRequest{
		Passphrase:            cfg.Passphrase,
		SerializedTransaction: revocationBytes,
		AdditionalScripts:     ticketScripts,
	}

	resp, err := wc.wsvc.SignTransaction(ctx, req)
	if err != nil {
		return err
	}

	signed := wire.NewMsgTx()
	signed.FromBytes(resp.Transaction)

	if signed.TxIn[0].SignatureScript == nil {
		return ErrNoInputSignedOnRevocation
	}

	session.revocationScriptSig = signed.TxIn[0].SignatureScript

	return nil
}

func (wc *WalletClient) fixNonWalletSplitInputs(splitCopy *wire.MsgTx, session *BuyerSession, cfg *BuyerConfig) []*pb.SignTransactionRequest_AdditionalScript {
	walletSplitInputs := make(map[wire.OutPoint]bool)
	for _, in := range session.splitInputs {
		walletSplitInputs[in.PreviousOutPoint] = true
	}

	dummyScripts := make([]*pb.SignTransactionRequest_AdditionalScript, 0, len(splitCopy.TxIn)-len(session.splitInputs))
	pkScript, scriptSig := dummyScriptSigner(cfg.ChainParams)
	for _, in := range splitCopy.TxIn {
		if _, fromWallet := walletSplitInputs[in.PreviousOutPoint]; fromWallet {
			continue
		}

		sc := &pb.SignTransactionRequest_AdditionalScript{
			TransactionHash: in.PreviousOutPoint.Hash[:],
			OutputIndex:     in.PreviousOutPoint.Index,
			Tree:            int32(in.PreviousOutPoint.Tree),
			PkScript:        pkScript,
		}
		dummyScripts = append(dummyScripts, sc)
		in.SignatureScript = scriptSig
	}

	return dummyScripts

}

func (wc *WalletClient) SignSplit(ctx context.Context, session *BuyerSession, cfg *BuyerConfig) error {

	splitCopy := session.splitTx.Copy()

	// fix the inputs that are not from this wallet, so that the wallet doesn't
	// error out saying it couldn't find the given pkscript. Not the ideal way
	// to go about this (ideally we should retrieve the actual pkscripts from the
	// network and let the wallet error out on signing)
	dummyScripts := wc.fixNonWalletSplitInputs(splitCopy, session, cfg)

	splitBytes, err := splitCopy.Bytes()
	if err != nil {
		return err
	}

	req := &pb.SignTransactionRequest{
		Passphrase:            cfg.Passphrase,
		SerializedTransaction: splitBytes,
		AdditionalScripts:     dummyScripts,
	}

	resp, err := wc.wsvc.SignTransaction(ctx, req)
	if err != nil {
		return err
	}

	signed := wire.NewMsgTx()
	signed.FromBytes(resp.Transaction)

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
		return ErrMissingSigOnSplitTx
	}

	return nil
}
