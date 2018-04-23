package daemon

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
	"golang.org/x/net/context"

	pb "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/matcherrpc"
)

func amountsToUint(amounts []dcrutil.Amount) []uint64 {
	res := make([]uint64, len(amounts))
	for i, a := range amounts {
		res[i] = uint64(a)
	}
	return res
}

func encodeQueueName(name string) string {
	hash := sha256.Sum256([]byte(name))
	return hex.EncodeToString(hash[:])
}

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

	watcher := make(chan []matcher.WaitingQueue)
	svc.matcher.WatchWaitingList(server.Context(), watcher)

	for {
		select {
		case <-server.Context().Done():
			return server.Context().Err()
		case queues := <-watcher:
			resp := &pb.WatchWaitingListResponse{
				Queues: make([]*pb.WatchWaitingListResponse_Queue, len(queues)),
			}
			for i, q := range queues {
				resp.Queues[i] = &pb.WatchWaitingListResponse_Queue{
					Name:    encodeQueueName(q.Name),
					Amounts: amountsToUint(q.Amounts),
				}
			}
			err := server.Send(resp)
			if err != nil {
				return err
			}
		}
	}
}

func (svc *SplitTicketMatcherService) FindMatches(ctx context.Context, req *pb.FindMatchesRequest) (*pb.FindMatchesResponse, error) {
	sess, err := svc.matcher.AddParticipant(ctx, req.Amount, req.SessionName)
	if err != nil {
		return nil, err
	}

	res := &pb.FindMatchesResponse{
		Amount:        uint64(sess.CommitAmount),
		Fee:           uint64(sess.Fee),
		SessionId:     int32(sess.ID),
		PoolFee:       uint64(sess.PoolFee),
		MainchainHash: sess.Session.MainchainHash[:],
		TicketPrice:   uint64(sess.Session.TicketPrice),
		TotalPoolFee:  uint64(sess.Session.PoolFee),
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

	if len(req.SecretnbHash) < matcher.SecretNbHashSize {
		return nil, ErrSecretNbSizeError
	}
	var secretNbHash matcher.SecretNumberHash
	copy(secretNbHash[:], req.SecretnbHash)

	split, ticketTempl, _, parts, err := svc.matcher.SetParticipantsOutputs(ctx, matcher.ParticipantID(req.SessionId),
		*commitTxout, *changeTxout, voteAddr, *splitChange, *splitTxout, splitOutpoints, poolAddr, secretNbHash)
	if err != nil {
		return nil, err
	}

	buffTicket, err := ticketTempl.Bytes()
	if err != nil {
		return nil, err
	}

	buffSplit, err := split.Bytes()
	if err != nil {
		return nil, err
	}

	partsResp := make([]*pb.GenerateTicketResponse_Participant, len(parts))
	for i, p := range parts {
		partsResp[i] = &pb.GenerateTicketResponse_Participant{
			SecretnbHash: p.SecretHash[:],
			VotePkScript: p.VotePkScript,
			PoolPkScript: p.PoolPkScript,
			Amount:       uint64(p.Amount),
		}
	}

	resp := &pb.GenerateTicketResponse{
		SplitTx:        buffSplit,
		TicketTemplate: buffTicket,
		Participants:   partsResp,
	}

	return resp, nil
}

func (svc *SplitTicketMatcherService) FundTicket(ctx context.Context, req *pb.FundTicketRequest) (*pb.FundTicketResponse, error) {

	ticketsInput := make([][]byte, len(req.Tickets))
	for i, t := range req.Tickets {
		ticketsInput[i] = t.TicketInputScriptsig
	}

	tickets, revocations, err := svc.matcher.FundTicket(ctx, matcher.ParticipantID(req.SessionId),
		ticketsInput, req.RevocationScriptSig)
	if err != nil {
		return nil, err
	}

	respTickets := make([]*pb.FundTicketResponse_FundedParticipantTicket, len(tickets))
	for i := range tickets {
		respTickets[i] = &pb.FundTicketResponse_FundedParticipantTicket{
			Ticket:     tickets[i],
			Revocation: revocations[i],
		}
	}

	resp := &pb.FundTicketResponse{
		Tickets: respTickets,
	}
	return resp, nil
}

func (svc *SplitTicketMatcherService) FundSplitTx(ctx context.Context, req *pb.FundSplitTxRequest) (*pb.FundSplitTxResponse, error) {
	split, ticket, revocation, secrets, err := svc.matcher.FundSplit(ctx,
		matcher.ParticipantID(req.SessionId),
		req.SplitTxScriptsigs, matcher.SecretNumber(req.Secretnb))
	if err != nil {
		return nil, err
	}

	respSecrets := make([]*pb.FundSplitTxResponse_ParticipantSecret, len(secrets))
	for i, s := range secrets {
		respSecrets[i] = &pb.FundSplitTxResponse_ParticipantSecret{
			Secretnb: uint64(s),
		}
	}

	resp := &pb.FundSplitTxResponse{
		SplitTx:        split,
		SelectedTicket: ticket,
		Revocation:     revocation,
		Secrets:        respSecrets,
	}
	return resp, nil
}

func (svc *SplitTicketMatcherService) Status(context.Context, *pb.StatusRequest) (*pb.StatusResponse, error) {
	return &pb.StatusResponse{
		TicketPrice: svc.priceProvider.CurrentTicketPrice(),
	}, nil
}
