package daemon

import (
	"bytes"

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

	var commitTxout, changeTxout *wire.TxOut
	var voteAddr dcrutil.Address
	var err error

	commitTxout = wire.NewTxOut(int64(req.CommitmentOutput.Value), req.CommitmentOutput.Script)
	changeTxout = wire.NewTxOut(int64(req.ChangeOutput.Value), req.ChangeOutput.Script)
	voteAddr, err = dcrutil.DecodeAddress(req.VoteAddress)
	if err != nil {
		return nil, err
	}

	tx, outIdx, err := svc.matcher.SetParticipantsOutputs(matcher.SessionID(req.SessionId),
		*commitTxout, *changeTxout, voteAddr)
	if err != nil {
		return nil, err
	}

	buff := bytes.NewBuffer(nil)
	buff.Grow(tx.SerializeSize())
	err = tx.BtcEncode(buff, 0)
	if err != nil {
		return nil, err
	}

	resp := &pb.GenerateTicketResponse{
		Transaction: buff.Bytes(),
		OutputIndex: int32(outIdx),
	}

	return resp, nil
}

func (svc *SplitTicketMatcherService) PublishTicket(ctx context.Context, req *pb.PublishTicketRequest) (*pb.PublishTicketResponse, error) {

	var splitTx *wire.MsgTx
	var input *wire.TxIn

	splitBuff := bytes.NewBuffer(req.GetSplitTx())
	splitTx = wire.NewMsgTx()
	splitTx.BtcDecode(splitBuff, 0)

	inputOutpoint := &wire.OutPoint{
		Hash:  splitTx.TxHash(),
		Index: uint32(req.SplitTxOutputIndex),
		Tree:  wire.TxTreeRegular,
	}
	input = wire.NewTxIn(inputOutpoint, req.GetTicketInputScriptsig())

	ticket, err := svc.matcher.PublishTransaction(matcher.SessionID(req.SessionId), splitTx,
		int(req.SplitTxOutputIndex), input)
	if err != nil {
		return nil, err
	}

	buff := bytes.NewBuffer(nil)
	buff.Grow(ticket.SerializeSize())
	err = ticket.BtcEncode(buff, 0)
	if err != nil {
		return nil, err
	}

	resp := &pb.PublishTicketResponse{
		TicketTx: buff.Bytes(),
	}
	return resp, nil
}

func (svc *SplitTicketMatcherService) Status(context.Context, *pb.StatusRequest) (*pb.StatusResponse, error) {
	return &pb.StatusResponse{
		TicketPrice: 666,
	}, nil
}
