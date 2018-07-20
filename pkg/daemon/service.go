package daemon

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"
	"github.com/pkg/errors"
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

// SplitTicketMatcherService implements the methods required to accept split
// ticket session commands from a grpc service.
type SplitTicketMatcherService struct {
	matcher            *matcher.Matcher
	networkProvider    matcher.NetworkProvider
	allowPublicSession bool
}

// NewSplitTicketMatcherService creates a new instance of a service, given all
// required options.
func NewSplitTicketMatcherService(matcher *matcher.Matcher,
	networkProvider matcher.NetworkProvider, allowPublicSession bool) *SplitTicketMatcherService {
	return &SplitTicketMatcherService{
		matcher:            matcher,
		networkProvider:    networkProvider,
		allowPublicSession: allowPublicSession,
	}
}

// WatchWaitingList fulfills SplitTicketMatcherServiceServer
func (svc *SplitTicketMatcherService) WatchWaitingList(req *pb.WatchWaitingListRequest, server pb.SplitTicketMatcherService_WatchWaitingListServer) error {

	watcher := make(chan []matcher.WaitingQueue)
	svc.matcher.WatchWaitingList(server.Context(), watcher, req.SendCurrent)

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

// FindMatches fulfills SplitTicketMatcherServiceServer
func (svc *SplitTicketMatcherService) FindMatches(ctx context.Context, req *pb.FindMatchesRequest) (*pb.FindMatchesResponse, error) {
	var voteAddr, poolAddr dcrutil.Address
	var err error

	if req.ProtocolVersion != pkg.ProtocolVersion {
		return nil, errors.Errorf("server is running a different protocol "+
			"version (%d) than client (%d)", pkg.ProtocolVersion,
			req.ProtocolVersion)
	}

	if req.SessionName == "" && !svc.allowPublicSession {
		return nil, errors.Errorf("server does not allow participation in " +
			"the public session")
	}

	if voteAddr, err = dcrutil.DecodeAddress(req.VoteAddress); err != nil {
		return nil, errors.Wrap(err, "error decoding vote address")
	}

	if poolAddr, err = dcrutil.DecodeAddress(req.PoolAddress); err != nil {
		return nil, errors.Wrap(err, "error decoding pool address")
	}

	sess, err := svc.matcher.AddParticipant(ctx, req.Amount, req.SessionName,
		voteAddr, poolAddr)
	if err != nil {
		return nil, err
	}

	res := &pb.FindMatchesResponse{
		Amount:          uint64(sess.CommitAmount),
		Fee:             uint64(sess.Fee),
		SessionId:       uint32(sess.ID),
		PoolFee:         uint64(sess.PoolFee),
		MainchainHash:   sess.Session.MainchainHash[:],
		MainchainHeight: sess.Session.MainchainHeight,
		TicketPrice:     uint64(sess.Session.TicketPrice),
		NbParticipants:  uint32(len(sess.Session.Participants)),
	}
	return res, nil
}

// GenerateTicket fulfills SplitTicketMatcherServiceServer
func (svc *SplitTicketMatcherService) GenerateTicket(ctx context.Context, req *pb.GenerateTicketRequest) (*pb.GenerateTicketResponse, error) {

	var splitChange *wire.TxOut
	var commitAddr, splitAddr dcrutil.Address
	var err error

	splitChange = wire.NewTxOut(int64(req.SplitTxChange.Value), req.SplitTxChange.Script)

	if commitAddr, err = dcrutil.DecodeAddress(req.CommitmentAddress); err != nil {
		return nil, errors.Wrap(err, "error decoding commitment address")
	}

	if splitAddr, err = dcrutil.DecodeAddress(req.SplitTxAddress); err != nil {
		return nil, errors.Wrap(err, "error decoding split tx address")
	}

	splitOutpoints := make([]*wire.OutPoint, len(req.SplitTxInputs))
	for i, in := range req.SplitTxInputs {
		var hash *chainhash.Hash
		hash, err = chainhash.NewHash(in.PrevHash)
		if err != nil {
			return nil, err
		}
		splitOutpoints[i] = wire.NewOutPoint(hash, uint32(in.PrevIndex), int8(in.Tree))
	}

	if len(req.SecretnbHash) < splitticket.SecretNbHashSize {
		return nil, errors.Errorf("secret hash sent does not have the " +
			"correct size")
	}
	var secretNbHash splitticket.SecretNumberHash
	copy(secretNbHash[:], req.SecretnbHash)

	split, ticketTempl, parts, partIndex, err := svc.matcher.SetParticipantsOutputs(ctx,
		matcher.ParticipantID(req.SessionId), commitAddr,
		splitAddr, splitChange, splitOutpoints, secretNbHash)
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
		Index:          partIndex,
	}

	return resp, nil
}

// FundTicket fulfills SplitTicketMatcherServiceServer
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

// FundSplitTx fulfills SplitTicketMatcherServiceServer
func (svc *SplitTicketMatcherService) FundSplitTx(ctx context.Context, req *pb.FundSplitTxRequest) (*pb.FundSplitTxResponse, error) {
	split, secrets, err := svc.matcher.FundSplit(ctx,
		matcher.ParticipantID(req.SessionId),
		req.SplitTxScriptsigs, splitticket.SecretNumber(req.Secretnb))
	if err != nil {
		return nil, err
	}

	respSecrets := make([]uint64, len(secrets))
	for i, s := range secrets {
		respSecrets[i] = uint64(s)
	}

	resp := &pb.FundSplitTxResponse{
		SplitTx:       split,
		SecretNumbers: respSecrets,
	}
	return resp, nil
}

// Status fulfills SplitTicketMatcherServiceServer
func (svc *SplitTicketMatcherService) Status(context.Context, *pb.StatusRequest) (*pb.StatusResponse, error) {
	return &pb.StatusResponse{
		TicketPrice: svc.networkProvider.CurrentTicketPrice(),
	}, nil
}
