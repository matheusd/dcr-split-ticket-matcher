package buyer

import (
	"context"
	"fmt"
	"io"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"

	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	pb "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/matcherrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type MatcherClient struct {
	client pb.SplitTicketMatcherServiceClient
	conn   *grpc.ClientConn
}

func ConnectToMatcherService(matcherHost string, certFile string) (*MatcherClient, error) {

	opt := grpc.WithInsecure()
	if certFile != "" {
		creds, err := credentials.NewClientTLSFromFile(certFile, "localhost")
		if err != nil {
			return nil, err
		}

		opt = grpc.WithTransportCredentials(creds)
	}

	conn, err := grpc.Dial(matcherHost, opt)
	if err != nil {
		return nil, err
	}

	client := pb.NewSplitTicketMatcherServiceClient(conn)

	mc := &MatcherClient{
		client: client,
		conn:   conn,
	}
	return mc, err
}

func (mc *MatcherClient) Status(ctx context.Context) (*pb.StatusResponse, error) {
	req := &pb.StatusRequest{}
	return mc.client.Status(ctx, req)
}

func (mc *MatcherClient) Participate(ctx context.Context, maxAmount dcrutil.Amount) (*BuyerSession, error) {
	req := &pb.FindMatchesRequest{Amount: uint64(maxAmount)}
	resp, err := mc.client.FindMatches(ctx, req)
	if err != nil {
		return nil, err
	}

	sess := &BuyerSession{
		ID:      matcher.ParticipantID(resp.SessionId),
		Amount:  dcrutil.Amount(resp.Amount),
		Fee:     dcrutil.Amount(resp.Fee),
		PoolFee: dcrutil.Amount(resp.PoolFee),
	}
	return sess, nil
}

func (mc *MatcherClient) GenerateTicket(ctx context.Context, session *BuyerSession, cfg *BuyerConfig) error {

	req := &pb.GenerateTicketRequest{
		SessionId: int32(session.ID),
		CommitmentOutput: &pb.TxOut{
			Script: session.ticketOutput.PkScript,
			Value:  uint64(session.ticketOutput.Value),
		},
		ChangeOutput: &pb.TxOut{
			Script: session.ticketChange.PkScript,
			Value:  uint64(session.ticketChange.Value),
		},
		SplitTxChange: &pb.TxOut{
			Script: session.splitChange.PkScript,
			Value:  uint64(session.splitChange.Value),
		},
		SplitTxOutput: &pb.TxOut{
			Script: session.splitOutput.PkScript,
			Value:  uint64(session.splitOutput.Value),
		},
		VoteAddress: cfg.VoteAddress,
		PoolAddress: cfg.PoolAddress,
	}

	req.SplitTxInputs = make([]*pb.OutPoint, len(session.splitInputs))
	for i, in := range session.splitInputs {
		req.SplitTxInputs[i] = &pb.OutPoint{
			PrevHash:  in.PreviousOutPoint.Hash.CloneBytes(),
			PrevIndex: int32(in.PreviousOutPoint.Index),
			Tree:      int32(in.PreviousOutPoint.Tree),
		}
	}

	resp, err := mc.client.GenerateTicket(ctx, req)
	if err != nil {
		return err
	}

	session.ticket = wire.NewMsgTx()
	err = session.ticket.FromBytes(resp.Ticket)
	if err != nil {
		return err
	}

	session.splitTx = wire.NewMsgTx()
	session.splitTx.FromBytes(resp.SplitTx)
	if err != nil {
		return err
	}

	session.revocation = wire.NewMsgTx()
	session.revocation.FromBytes(resp.Revocation)
	if err != nil {
		return err
	}

	voteOutput := session.ticket.TxOut[0]
	_, addresses, n, err := txscript.ExtractPkScriptAddrs(voteOutput.Version, voteOutput.PkScript, cfg.ChainParams)
	if err != nil {
		return err
	}

	if n != 1 {
		return ErrWrongNumOfAddressesInVoteOut
	}

	session.isVoter = addresses[0].String() == cfg.VoteAddress

	return nil
}

func (mc *MatcherClient) FundTicket(ctx context.Context, session *BuyerSession, cfg *BuyerConfig) error {

	req := &pb.FundTicketRequest{
		SessionId:            int32(session.ID),
		TicketInputScriptsig: session.ticketScriptSig,
		RevocationScriptSig:  session.revocationScriptSig,
	}

	resp, err := mc.client.FundTicket(ctx, req)
	if err != nil {
		return err
	}

	fundedTicket := wire.NewMsgTx()
	err = fundedTicket.FromBytes(resp.Ticket)
	if err != nil {
		return err
	}

	fundedRevocation := wire.NewMsgTx()
	err = fundedRevocation.FromBytes(resp.Revocation)
	if err != nil {
		return err
	}

	// TODO: verify if the funded ticket and revocation haven't changed and
	// that revocation actually revokes the given ticket

	session.ticket = fundedTicket
	session.revocation = fundedRevocation

	return nil
}

func (mc *MatcherClient) FundSplitTx(ctx context.Context, session *BuyerSession, cfg *BuyerConfig) error {

	splitTxSigs := make([][]byte, len(session.splitInputs))
	for i, in := range session.splitInputs {
		splitTxSigs[i] = in.SignatureScript
	}

	req := &pb.FundSplitTxRequest{
		SessionId:         int32(session.ID),
		SplitTxScriptsigs: splitTxSigs,
	}

	resp, err := mc.client.FundSplitTx(ctx, req)
	if err != nil {
		return err
	}

	fundedSplit := wire.NewMsgTx()
	err = fundedSplit.FromBytes(resp.SplitTx)
	if err != nil {
		return err
	}

	// TODO: verify if the funded split hasn't change and actually does
	// what it should

	session.splitTx = fundedSplit

	return nil
}

type waitingListWatcher interface {
	ListChanged([]dcrutil.Amount)
}

func (mc *MatcherClient) WatchWaitingList(ctx context.Context, watcher waitingListWatcher) error {
	req := &pb.WatchWaitingListRequest{}
	cli, err := mc.client.WatchWaitingList(ctx, req)
	if err != nil {
		return err
	}
	go func() {
		for {
			resp, err := cli.Recv()
			if err != nil {
				if err != io.EOF {
					fmt.Println("Error reading waiting list: %v", err)
				}
				return
			} else {
				amnts := make([]dcrutil.Amount, len(resp.Amounts))
				for i, a := range resp.Amounts {
					amnts[i] = dcrutil.Amount(a)
				}
				watcher.ListChanged(amnts)
			}
		}
	}()

	return nil
}

func (mc *MatcherClient) Close() {
	mc.conn.Close()
}
