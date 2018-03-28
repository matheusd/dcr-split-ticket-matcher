package daemon

import (
	"bytes"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
	"golang.org/x/net/context"

	pb "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/matcherrpc"
)

type SplitTicketMatcherService struct {
	matcher       *matcher.Matcher
	priceProvider matcher.TicketPriceProvider
}

func NewSplitTicketMatcherService(matcher *matcher.Matcher, priceProvider matcher.TicketPriceProvider) *SplitTicketMatcherService {
	return &SplitTicketMatcherService{
		matcher:       matcher,
		priceProvider: priceProvider,
	}
}

func (svc *SplitTicketMatcherService) WatchWaitingList(req *pb.WatchWaitingListRequest, server pb.SplitTicketMatcherService_WatchWaitingListServer) error {

	watcher := make(chan []dcrutil.Amount)
	svc.matcher.WatchWaitingList(server.Context(), watcher)

	for {
		select {
		case <-server.Context().Done():
			return server.Context().Err()
		case amounts := <-watcher:
			resp := &pb.WatchWaitingListResponse{
				Amounts: make([]uint64, len(amounts)),
			}
			for i, a := range amounts {
				resp.Amounts[i] = uint64(a)
			}
			err := server.Send(resp)
			if err != nil {
				return err
			}
		}
	}
}

func (svc *SplitTicketMatcherService) FindMatches(ctx context.Context, req *pb.FindMatchesRequest) (*pb.FindMatchesResponse, error) {
	sess, err := svc.matcher.AddParticipant(ctx, req.Amount)
	if err != nil {
		return nil, err
	}

	res := &pb.FindMatchesResponse{
		Amount:    uint64(sess.CommitAmount),
		Fee:       uint64(sess.Fee),
		SessionId: int32(sess.ID),
		PoolFee:   uint64(sess.PoolFee),
	}
	return res, nil
}

func (svc *SplitTicketMatcherService) GenerateTicket(ctx context.Context, req *pb.GenerateTicketRequest) (*pb.GenerateTicketResponse, error) {

	var commitTxout, changeTxout, splitTxout, splitChange *wire.TxOut
	var voteAddr, poolAddr dcrutil.Address
	var err error

	commitTxout = wire.NewTxOut(int64(req.CommitmentOutput.Value), req.CommitmentOutput.Script)
	changeTxout = wire.NewTxOut(int64(req.ChangeOutput.Value), req.ChangeOutput.Script)
	splitTxout = wire.NewTxOut(int64(req.SplitTxOutput.Value), req.SplitTxOutput.Script)
	splitChange = wire.NewTxOut(int64(req.SplitTxChange.Value), req.SplitTxChange.Script)
	voteAddr, err = dcrutil.DecodeAddress(req.VoteAddress)
	if err != nil {
		return nil, err
	}

	poolAddr, err = dcrutil.DecodeAddress(req.PoolAddress)
	if err != nil {
		return nil, err
	}

	splitOutpoints := make([]*wire.OutPoint, len(req.SplitTxInputs))
	for i, in := range req.SplitTxInputs {
		hash, err := chainhash.NewHash(in.PrevHash)
		if err != nil {
			return nil, err
		}
		splitOutpoints[i] = wire.NewOutPoint(hash, uint32(in.PrevIndex), int8(in.Tree))
	}

	ticket, split, revocation, err := svc.matcher.SetParticipantsOutputs(ctx, matcher.ParticipantID(req.SessionId),
		*commitTxout, *changeTxout, voteAddr, *splitChange, *splitTxout, splitOutpoints, poolAddr)
	if err != nil {
		return nil, err
	}

	buffTicket := bytes.NewBuffer(nil)
	buffTicket.Grow(ticket.SerializeSize())
	err = ticket.BtcEncode(buffTicket, 0)
	if err != nil {
		return nil, err
	}

	buffSplit := bytes.NewBuffer(nil)
	buffSplit.Grow(split.SerializeSize())
	err = split.BtcEncode(buffSplit, 0)
	if err != nil {
		return nil, err
	}

	buffRevoke := bytes.NewBuffer(nil)
	buffRevoke.Grow(revocation.SerializeSize())
	err = revocation.BtcEncode(buffRevoke, 0)
	if err != nil {
		return nil, err
	}

	resp := &pb.GenerateTicketResponse{
		Ticket:     buffTicket.Bytes(),
		SplitTx:    buffSplit.Bytes(),
		Revocation: buffRevoke.Bytes(),
	}

	return resp, nil
}

func (svc *SplitTicketMatcherService) FundTicket(ctx context.Context, req *pb.FundTicketRequest) (*pb.FundTicketResponse, error) {
	ticket, revocation, err := svc.matcher.FundTicket(ctx, matcher.ParticipantID(req.SessionId), req.TicketInputScriptsig, req.RevocationScriptSig)
	if err != nil {
		return nil, err
	}

	buffTicket := bytes.NewBuffer(nil)
	buffTicket.Grow(ticket.SerializeSize())
	err = ticket.BtcEncode(buffTicket, 0)
	if err != nil {
		return nil, err
	}

	buffRevocation := bytes.NewBuffer(nil)
	buffRevocation.Grow(ticket.SerializeSize())
	err = revocation.BtcEncode(buffRevocation, 0)
	if err != nil {
		return nil, err
	}

	resp := &pb.FundTicketResponse{
		Ticket:     buffTicket.Bytes(),
		Revocation: buffRevocation.Bytes(),
	}
	return resp, nil
}

func (svc *SplitTicketMatcherService) FundSplitTx(ctx context.Context, req *pb.FundSplitTxRequest) (*pb.FundSplitTxResponse, error) {
	split, err := svc.matcher.FundSplit(ctx, matcher.ParticipantID(req.SessionId),
		req.SplitTxScriptsigs)
	if err != nil {
		return nil, err
	}

	buffSplit := bytes.NewBuffer(nil)
	buffSplit.Grow(split.SerializeSize())
	err = split.BtcEncode(buffSplit, 0)
	if err != nil {
		return nil, err
	}

	resp := &pb.FundSplitTxResponse{
		SplitTx: buffSplit.Bytes(),
	}
	return resp, nil
}

func (svc *SplitTicketMatcherService) Status(context.Context, *pb.StatusRequest) (*pb.StatusResponse, error) {
	return &pb.StatusResponse{
		TicketPrice: svc.priceProvider.CurrentTicketPrice(),
	}, nil
}
