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
	matcher *matcher.Matcher
}

func NewSplitTicketMatcherService(matcher *matcher.Matcher) *SplitTicketMatcherService {
	return &SplitTicketMatcherService{
		matcher: matcher,
	}
}

func (svc *SplitTicketMatcherService) FindMatches(ctx context.Context, req *pb.FindMatchesRequest) (*pb.FindMatchesResponse, error) {
	sess, err := svc.matcher.AddParticipant(req.Amount)
	if err != nil {
		return nil, err
	}

	res := &pb.FindMatchesResponse{
		Amount:    uint64(sess.Amount),
		Fee:       uint64(sess.Fee),
		SessionId: int32(sess.ID),
	}
	return res, nil
}

func (svc *SplitTicketMatcherService) GenerateTicket(ctx context.Context, req *pb.GenerateTicketRequest) (*pb.GenerateTicketResponse, error) {

	var commitTxout, changeTxout, splitTxout, splitChange *wire.TxOut
	var voteAddr dcrutil.Address
	var err error

	commitTxout = wire.NewTxOut(int64(req.CommitmentOutput.Value), req.CommitmentOutput.Script)
	changeTxout = wire.NewTxOut(int64(req.ChangeOutput.Value), req.ChangeOutput.Script)
	splitTxout = wire.NewTxOut(int64(req.SplitTxOutput.Value), req.SplitTxOutput.Script)
	splitChange = wire.NewTxOut(int64(req.SplitTxChange.Value), req.SplitTxChange.Script)
	voteAddr, err = dcrutil.DecodeAddress(req.VoteAddress)
	if err != nil {
		return nil, err
	}

	splitOutpoints := make([]*wire.OutPoint, len(req.SplitTxInputs))
	for i, in := range req.SplitTxInputs {
		hash, err := chainhash.NewHash(in.PrevHash)
		if err != nil {
			return nil, err
		}
		splitOutpoints[i] = wire.NewOutPoint(hash, uint32(in.PrevIndex), wire.TxTreeRegular)
	}

	ticket, split, err := svc.matcher.SetParticipantsOutputs(matcher.SessionID(req.SessionId),
		*commitTxout, *changeTxout, voteAddr, *splitChange, *splitTxout, splitOutpoints)
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

	resp := &pb.GenerateTicketResponse{
		Ticket:  buffTicket.Bytes(),
		SplitTx: buffSplit.Bytes(),
	}

	return resp, nil
}

func (svc *SplitTicketMatcherService) FundTicket(ctx context.Context, req *pb.FundTicketRequest) (*pb.FundTicketResponse, error) {
	ticket, err := svc.matcher.FundTicket(matcher.SessionID(req.SessionId), req.TicketInputScriptsig)
	if err != nil {
		return nil, err
	}

	buffTicket := bytes.NewBuffer(nil)
	buffTicket.Grow(ticket.SerializeSize())
	err = ticket.BtcEncode(buffTicket, 0)
	if err != nil {
		return nil, err
	}

	resp := &pb.FundTicketResponse{
		Ticket: buffTicket.Bytes(),
	}
	return resp, nil
}

func (svc *SplitTicketMatcherService) FundSplitTx(ctx context.Context, req *pb.FundSplitTxRequest) (*pb.FundSplitTxResponse, error) {
	split, err := svc.matcher.FundSplit(matcher.SessionID(req.SessionId),
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
		TicketPrice: 666,
	}, nil
}
