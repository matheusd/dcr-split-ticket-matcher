package buyer

import (
	"bytes"
	"context"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"

	pb "github.com/decred/dcrwallet/rpc/walletrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type WalletClient struct {
	conn *grpc.ClientConn
	wsvc pb.WalletServiceClient
}

func ConenctToWallet(walletHost string, walletCert string) (*WalletClient, error) {
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

func (wc *WalletClient) GenerateOutputs(ctx context.Context, session *BuyerSession, cfg *BuyerConfig) error {
	splitOutAddr, ticketOutAdd, err := wc.generateOutputAddresses(ctx, session, cfg)

	zeroed := [20]byte{}
	addrZeroed, err := dcrutil.NewAddressPubKeyHash(zeroed[:], cfg.ChainParams, 0)
	if err != nil {
		return err
	}

	ticketCommitScript, err := txscript.GenerateSStxAddrPush(ticketOutAdd,
		session.Amount+session.Fee, cfg.SStxFeeLimits)
	if err != nil {
		return err
	}

	ticketChangeScript, err := txscript.PayToSStxChange(addrZeroed)
	if err != nil {
		return err
	}

	splitOutScript, err := txscript.PayToAddrScript(splitOutAddr)
	if err != nil {
		return err
	}

	session.ticketOutput = wire.NewTxOut(0, ticketCommitScript)
	session.ticketChange = wire.NewTxOut(0, ticketChangeScript)
	session.splitOutput = wire.NewTxOut(int64(session.Amount+session.Fee), splitOutScript)

	return wc.generateSplitTxInputs(ctx, session, cfg)
}

func (wc *WalletClient) generateOutputAddresses(ctx context.Context, session *BuyerSession, cfg *BuyerConfig) (
	splitOut, ticketOut dcrutil.Address, err error) {

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

	return
}

func (wc *WalletClient) generateSplitTxInputs(ctx context.Context, session *BuyerSession, cfg *BuyerConfig) error {

	// FIXME probably missing the split tx fee
	fundAmount := int64(session.Amount + session.Fee + session.PoolFee)
	rep := reporterFromContext(ctx)

	req := &pb.FundTransactionRequest{
		Account:                  cfg.SourceAccount,
		TargetAmount:             fundAmount,
		RequiredConfirmations:    1,
		IncludeImmatureCoinbases: false,
		IncludeChangeScript:      true,
	}

	rep.reportStage(ctx, StageGenerateSplitInputs, session, cfg)
	resp, err := wc.wsvc.FundTransaction(ctx, req)
	if err != nil {
		return err
	}

	if resp.TotalAmount < req.TargetAmount {
		return ErrNotEnoughFundsFundSplit
	}

	changeAmount := resp.TotalAmount - req.TargetAmount
	session.splitChange = wire.NewTxOut(changeAmount, resp.ChangePkScript)

	session.splitInputs = make([]*wire.TxIn, len(resp.SelectedOutputs))
	for i, o := range resp.SelectedOutputs {
		hash, err := chainhash.NewHash(o.TransactionHash)
		if err != nil {
			return err
		}

		outp := wire.NewOutPoint(hash, o.OutputIndex, int8(o.Tree))
		session.splitInputs[i] = wire.NewTxIn(outp, nil)
	}

	return nil
}

func (wc *WalletClient) SignTicket(ctx context.Context, session *BuyerSession, cfg *BuyerConfig) error {

	ticketBytes, err := session.ticket.Bytes()
	if err != nil {
		return err
	}

	splitTxHash := session.splitTx.TxHash()

	splitScripts := make([]*pb.SignTransactionRequest_AdditionalScript, len(session.splitTx.TxOut))
	for i, out := range session.splitTx.TxOut {
		splitScripts[i] = &pb.SignTransactionRequest_AdditionalScript{
			OutputIndex:     uint32(i),
			PkScript:        out.PkScript,
			TransactionHash: splitTxHash[:],
			Tree:            int32(wire.TxTreeRegular),
		}
	}

	req := &pb.SignTransactionRequest{
		Passphrase:            cfg.Passphrase,
		SerializedTransaction: ticketBytes,
		AdditionalScripts:     splitScripts,
	}

	resp, err := wc.wsvc.SignTransaction(ctx, req)
	if err != nil {
		return err
	}

	signed := wire.NewMsgTx()
	signed.FromBytes(resp.Transaction)

	for _, in := range signed.TxIn {
		if in.SignatureScript != nil {
			session.ticketScriptSig = in.SignatureScript
			return nil
		}
	}

	return ErrNoInputSignedOnTicket
}

func (wc *WalletClient) SignRevocation(ctx context.Context, session *BuyerSession, cfg *BuyerConfig) error {

	revocationBytes, err := session.revocation.Bytes()
	if err != nil {
		return err
	}

	ticketHash := session.ticket.TxHash()

	ticketScripts := make([]*pb.SignTransactionRequest_AdditionalScript, 1)
	ticketScripts[0] = &pb.SignTransactionRequest_AdditionalScript{
		OutputIndex:     0,
		PkScript:        session.ticket.TxOut[0].PkScript,
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

func (wc *WalletClient) SignSplit(ctx context.Context, session *BuyerSession, cfg *BuyerConfig) error {

	splitBytes, err := session.splitTx.Bytes()
	if err != nil {
		return err
	}

	req := &pb.SignTransactionRequest{
		Passphrase:            cfg.Passphrase,
		SerializedTransaction: splitBytes,
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
			} else {
				return ErrWrongInputSignedOnSplit
			}
		}
	}

	if signedCount != len(session.splitInputs) {
		return ErrMissingSigOnSplitTx
	}

	return nil
}
