package buyer

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/ansel1/merry"
	"github.com/pkg/errors"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/validations"

	"github.com/decred/dcrd/chaincfg/chainhash"
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

func (mc *MatcherClient) Participate(ctx context.Context, maxAmount dcrutil.Amount, sessionName string) (*BuyerSession, error) {
	req := &pb.FindMatchesRequest{
		Amount:      uint64(maxAmount),
		SessionName: sessionName,
	}

	resp, err := mc.client.FindMatches(ctx, req)
	if err != nil {
		return nil, err
	}

	mainchainHash, err := chainhash.NewHash(resp.MainchainHash)
	if err != nil {
		return nil, err
	}

	// TODO: check if mainchainHash really is the mainchain tip at this moment
	// TODO: check if the ticket price really is the current ticket price according to the latest block

	sess := &BuyerSession{
		ID:            matcher.ParticipantID(resp.SessionId),
		Amount:        dcrutil.Amount(resp.Amount),
		Fee:           dcrutil.Amount(resp.Fee),
		PoolFee:       dcrutil.Amount(resp.PoolFee),
		TotalPoolFee:  dcrutil.Amount(resp.TotalPoolFee),
		TicketPrice:   dcrutil.Amount(resp.TicketPrice),
		mainchainHash: mainchainHash,
	}
	return sess, nil
}

func (mc *MatcherClient) GenerateTicket(ctx context.Context, session *BuyerSession, cfg *BuyerConfig) error {

	session.secretNb = matcher.SecretNumber(matcher.MustRandUint64())
	session.secretNbHash = session.secretNb.Hash(*session.mainchainHash)

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
		VoteAddress:  cfg.VoteAddress,
		PoolAddress:  cfg.PoolAddress,
		SecretnbHash: session.secretNbHash[:],
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

	session.ticketTemplate = wire.NewMsgTx()
	err = session.ticketTemplate.FromBytes(resp.TicketTemplate)
	if err != nil {
		return err
	}

	session.splitTx = wire.NewMsgTx()
	session.splitTx.FromBytes(resp.SplitTx)
	if err != nil {
		return err
	}

	if err = validations.CheckSplit(session.splitTx, cfg.ChainParams); err != nil {
		return err
	}

	secretHashes := make([]matcher.SecretNumberHash, len(resp.Participants))
	gotMyHash := false

	session.participants = make([]buyerSessionParticipant, len(resp.Participants))
	for i, p := range resp.Participants {
		session.participants[i] = buyerSessionParticipant{
			amount:       dcrutil.Amount(p.Amount),
			poolPkScript: p.PoolPkScript,
			votePkScript: p.VotePkScript,
		}
		copy(session.participants[i].secretHash[:], p.SecretnbHash)
		secretHashes[i] = session.participants[i].secretHash
		gotMyHash = gotMyHash || secretHashes[i].Equals(session.secretNbHash)
	}

	targetVoterHash := matcher.SecretNumberHashesHash(secretHashes, *session.mainchainHash)
	if (len(session.splitTx.TxOut) < 1) || (len(session.splitTx.TxOut[0].PkScript) < 1) || (session.splitTx.TxOut[0].PkScript[0] != txscript.OP_RETURN) {
		return ErrSplitTxOutZeroNotOpReturn
	}

	// pick the range [2:] because the first byte is the OP_RETURN, the second
	// is the push data op
	splitVoterCommitment := session.splitTx.TxOut[0].PkScript[2:]
	if !bytes.Equal(targetVoterHash, splitVoterCommitment) {
		return ErrWrongSplitTxVoterSelCommitment
	}

	// vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv
	// TODO: everything below this points doesn't really belong here (should be
	// calculated when signing my ticket/revocation)

	voteAddr, err := dcrutil.DecodeAddress(cfg.VoteAddress)
	if err != nil {
		return err
	}

	poolAddr, err := dcrutil.DecodeAddress(cfg.PoolAddress)
	if err != nil {
		return err
	}

	voteScript, err := txscript.PayToSStx(voteAddr)
	if err != nil {
		return err
	}

	var limits uint16 = 0x5800 // TODO: grab from the original template
	poolScript, err := txscript.GenerateSStxAddrPush(poolAddr, session.TotalPoolFee, limits)
	if err != nil {
		return err
	}

	session.votePkScript = voteScript
	session.poolPkScript = poolScript

	return nil
}

func (mc *MatcherClient) FundTicket(ctx context.Context, session *BuyerSession, cfg *BuyerConfig) error {

	var err error

	tickets := make([]*pb.FundTicketRequest_FundedParticipantTicket, len(session.ticketsScriptSig))
	for i, s := range session.ticketsScriptSig {
		tickets[i] = &pb.FundTicketRequest_FundedParticipantTicket{
			TicketInputScriptsig: s,
		}
	}

	req := &pb.FundTicketRequest{
		SessionId:           int32(session.ID),
		Tickets:             tickets,
		RevocationScriptSig: session.revocationScriptSig,
	}

	resp, err := mc.client.FundTicket(ctx, req)
	if err != nil {
		return err
	}

	if len(resp.Tickets) != len(session.participants) {
		return fmt.Errorf("Matcher replied with different number of tickets than participants")
	}

	partsAmounts := make([]dcrutil.Amount, len(session.participants))
	for i, p := range session.participants {
		partsAmounts[i] = p.amount
	}

	splitTx := session.splitTx
	for i, t := range resp.Tickets {

		ticket := wire.NewMsgTx()
		if err = ticket.FromBytes(t.Ticket); err != nil {
			return errors.Wrapf(err, "error decoding ticket bytes for part %d", i)
		}

		revocation := wire.NewMsgTx()
		if err = revocation.FromBytes(t.Revocation); err != nil {
			return errors.Wrapf(err, "error decoding reovaction bytes for part %d", i)
		}

		err = validations.CheckTicket(splitTx, ticket, session.TicketPrice,
			session.TotalPoolFee, session.Fee, partsAmounts, cfg.ChainParams)
		if err != nil {
			return errors.Wrapf(err, "error checking validity of ticket of part %d", i)
		}

		if err = validations.CheckRevocation(ticket, revocation, cfg.ChainParams); err != nil {
			return errors.Wrapf(err, "error checking validity of revocation of part %d", i)
		}

		session.participants[i].ticket = ticket
		session.participants[i].revocation = revocation
	}

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
		Secretnb:          uint64(session.secretNb),
	}

	resp, err := mc.client.FundSplitTx(ctx, req)
	if err != nil {
		return err
	}

	fundedSplit := wire.NewMsgTx()
	err = fundedSplit.FromBytes(resp.SplitTx)
	if err != nil {
		return merry.Prepend(err, "error decoding funded split")
	}

	selectedTicket := wire.NewMsgTx()
	err = selectedTicket.FromBytes(resp.SelectedTicket)
	if err != nil {
		return merry.Prepend(err, "error decoding selected ticket")
	}

	selectedRevocation := wire.NewMsgTx()
	err = selectedRevocation.FromBytes(resp.Revocation)
	if err != nil {
		return merry.Prepend(err, "error decoding selected revocation")
	}

	// TODO: verify if the funded split hasn't changed

	if err = validations.CheckSplit(fundedSplit, cfg.ChainParams); err != nil {
		return err
	}

	if len(resp.Secrets) != len(session.participants) {
		return merry.New("len(secrets) != len(participants)")
	}

	for i, s := range resp.Secrets {
		session.participants[i].secretNb = matcher.SecretNumber(s.Secretnb)
		sentSecretHash := session.participants[i].secretNb.Hash(*session.mainchainHash)
		if !sentSecretHash.Equals(session.participants[i].secretHash) {
			// TODO: show big red warning, as sending a wrong secret number
			// is a voting manipulation attempt
			return ErrWrongSecretNbProvided
		}
	}

	session.voterIndex = session.findVoterIndex()
	if session.voterIndex < 0 {
		return merry.New("Negative voter index")
	}

	// verify if the voting address for the submitted ticket actually is
	// for the given voter index. If these are different, that means the matcher
	// is malicious and is not honoring the deterministically selected voter.
	// TODO: show a big red warning.
	// TODO: check the pool fee destination output as well
	targetVoterPkScript := session.participants[session.voterIndex].votePkScript
	if !bytes.Equal(selectedTicket.TxOut[0].PkScript, targetVoterPkScript) {
		return merry.Errorf("DAAAAANGER!!!! Received funded ticket is not for the deterministically selected voter (%d)", session.voterIndex)
	}

	// TODO: verify if the published ticket transaction (received from the network)
	// actually is for the given voter index. Probably need to alert dcrd to
	// watch for transactions involving all voting addresses and alert on any
	// published that is not for the given ticket

	// TODO: we don't really need to receive the selected ticket from the
	// network as we should already have it and the revocation from the
	// previous step.

	session.fundedSplitTx = fundedSplit
	session.selectedTicket = selectedTicket
	session.selectedRevocation = selectedRevocation

	return nil
}

type waitingListWatcher interface {
	ListChanged([]matcher.WaitingQueue)
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
				queues := make([]matcher.WaitingQueue, len(resp.Queues))
				for i, q := range resp.Queues {
					queues[i] = matcher.WaitingQueue{
						Name:    q.Name,
						Amounts: uint64sToAmounts(q.Amounts),
					}
				}
				watcher.ListChanged(queues)
			}
		}
	}()

	return nil
}

func (mc *MatcherClient) Close() {
	mc.conn.Close()
}
