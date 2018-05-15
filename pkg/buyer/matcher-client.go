package buyer

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
	pb "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/matcherrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type MatcherClient struct {
	client  pb.SplitTicketMatcherServiceClient
	conn    *grpc.ClientConn
	network *decredNetwork
}

func ConnectToMatcherService(matcherHost string, certFile string, netCfg *decredNetworkConfig) (*MatcherClient, error) {

	network, err := connectToDecredNode(netCfg)
	if err != nil {
		return nil, errors.Wrapf(err, "error connecting to dcrd")
	}

	opt := grpc.WithInsecure()
	if certFile != "" {
		creds, err := credentials.NewClientTLSFromFile(certFile, "localhost")
		if err != nil {
			return nil, errors.Wrapf(err, "error creating credentials")
		}

		opt = grpc.WithTransportCredentials(creds)
	}

	conn, err := grpc.Dial(matcherHost, opt)
	if err != nil {
		return nil, errors.Wrapf(err, "error connecting to matcher host")
	}

	client := pb.NewSplitTicketMatcherServiceClient(conn)

	mc := &MatcherClient{
		client:  client,
		conn:    conn,
		network: network,
	}
	return mc, err
}

func (mc *MatcherClient) Status(ctx context.Context) (*pb.StatusResponse, error) {
	req := &pb.StatusRequest{}
	return mc.client.Status(ctx, req)
}

func (mc *MatcherClient) Participate(ctx context.Context, maxAmount dcrutil.Amount, sessionName string) (*BuyerSession, error) {
	req := &pb.FindMatchesRequest{
		Amount:          uint64(maxAmount),
		SessionName:     sessionName,
		ProtocolVersion: pkg.ProtocolVersion,
	}

	resp, err := mc.client.FindMatches(ctx, req)
	if err != nil {
		return nil, err
	}

	mainchainHash, err := chainhash.NewHash(resp.MainchainHash)
	if err != nil {
		return nil, err
	}

	sess := &BuyerSession{
		ID:              matcher.ParticipantID(resp.SessionId),
		Amount:          dcrutil.Amount(resp.Amount),
		Fee:             dcrutil.Amount(resp.Fee),
		PoolFee:         dcrutil.Amount(resp.PoolFee),
		TicketPrice:     dcrutil.Amount(resp.TicketPrice),
		mainchainHash:   mainchainHash,
		mainchainHeight: resp.MainchainHeight,
		nbParticipants:  resp.NbParticipants,
	}
	return sess, nil
}

func (mc *MatcherClient) GenerateTicket(ctx context.Context, session *BuyerSession, cfg *BuyerConfig) error {

	voteAddr, err := dcrutil.DecodeAddress(cfg.VoteAddress)
	if err != nil {
		return err
	}

	poolAddr, err := dcrutil.DecodeAddress(cfg.PoolAddress)
	if err != nil {
		return err
	}

	session.secretNb = splitticket.SecretNumber(matcher.MustRandUint64())
	session.secretNbHash = session.secretNb.Hash(session.mainchainHash)
	session.voteAddress = voteAddr
	session.poolAddress = poolAddr

	req := &pb.GenerateTicketRequest{
		SessionId: uint32(session.ID),
		SplitTxChange: &pb.TxOut{
			Script: session.splitChange.PkScript,
			Value:  uint64(session.splitChange.Value),
		},
		VoteAddress:       session.voteAddress.String(),
		PoolAddress:       session.poolAddress.String(),
		CommitmentAddress: session.ticketOutputAddress.String(),
		SplitTxAddress:    session.splitOutputAddress.String(),
		SecretnbHash:      session.secretNbHash[:],
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

	if uint32(len(resp.Participants)) != session.nbParticipants {
		return errors.Errorf("service returned information for a different "+
			"number of participants (%d) than expected (%d)",
			len(resp.Participants), session.nbParticipants)
	}

	if session.myIndex >= uint32(len(resp.Participants)) {
		return errors.Errorf("service returned an index (%d) larger than "+
			"the number of participants (%d)", session.myIndex,
			len(resp.Participants))
	}

	session.myIndex = resp.Index
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

	// cache the utxo map of the split so we can check for the validity of the tx
	utxoMap, err := splitticket.UtxoMapFromNetwork(mc.network.client, session.splitTx)
	if err != nil {
		return err
	}
	session.splitTxUtxoMap = utxoMap

	session.participants = make([]buyerSessionParticipant, len(resp.Participants))
	for i, p := range resp.Participants {
		session.participants[i] = buyerSessionParticipant{
			amount:       dcrutil.Amount(p.Amount),
			poolPkScript: p.PoolPkScript,
			votePkScript: p.VotePkScript,
		}
		copy(session.participants[i].secretHash[:], p.SecretnbHash)
	}

	myIdx := session.myIndex
	myPart := session.participants[myIdx]
	if !session.secretNbHash.Equals(myPart.secretHash) {
		return errors.Errorf("secret hash specified for participant %d does "+
			"not equal our hash", myIdx)
	}

	if myPart.amount != session.Amount {
		return errors.Errorf("amount specified for participant %d (%s) does "+
			"not equal our previous contribution amount (%s)", myIdx,
			myPart.amount, session.Amount)
	}

	// ensure the vote/pool scripts provided at my index are actually my own
	err = splitticket.CheckTicketScriptMatchAddresses(session.voteAddress,
		session.poolAddress, myPart.votePkScript, myPart.poolPkScript,
		session.PoolFee*dcrutil.Amount(len(session.participants)),
		cfg.ChainParams)
	if err != nil {
		return errors.Wrapf(err, "error checking the vote/pool scripts of my "+
			"ticket")
	}

	// ensure the split tx is valid
	err = splitticket.CheckSplit(session.splitTx, session.splitTxUtxoMap,
		session.secretHashes(), session.mainchainHash, session.mainchainHeight,
		cfg.ChainParams)
	if err != nil {
		return errors.Wrapf(err, "error checking split tx")
	}

	// ensure the ticket template is valid
	err = splitticket.CheckTicket(session.splitTx, session.ticketTemplate,
		session.TicketPrice, session.PoolFee, session.Fee, session.amounts(),
		session.mainchainHeight, cfg.ChainParams)
	if err != nil {
		return errors.Wrapf(err, "error checking ticket template")
	}

	// ensure my commitment, inputs and change is in the ticket/split
	err = splitticket.CheckParticipantInTicket(session.splitTx,
		session.ticketTemplate, session.Amount, session.Fee,
		session.ticketOutputAddress, session.splitOutputAddress,
		session.splitChange, session.myIndex, session.splitInputOutpoints(),
		cfg.ChainParams)
	if err != nil {
		return errors.Wrapf(err, "error checking my participation in ticket "+
			"template")
	}

	// ensure my participation in the split is correct
	err = splitticket.CheckParticipantInSplit(session.splitTx,
		session.splitOutputAddress, session.Amount, session.Fee,
		session.splitChange, cfg.ChainParams)
	if err != nil {
		return errors.Wrapf(err, "error checking my participation in split")
	}

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
		SessionId:           uint32(session.ID),
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

		err = splitticket.CheckTicket(splitTx, ticket, session.TicketPrice,
			session.PoolFee, session.Fee, partsAmounts, session.mainchainHeight,
			cfg.ChainParams)
		if err != nil {
			return errors.Wrapf(err, "error checking validity of ticket of part %d", i)
		}

		err = splitticket.CheckSignedTicket(splitTx, ticket, cfg.ChainParams)
		if err != nil {
			return errors.Wrapf(err, "error checking validity of signatures "+
				"of ticket of part %d", i)
		}

		if err = splitticket.CheckRevocation(ticket, revocation, cfg.ChainParams); err != nil {
			return errors.Wrapf(err, "error checking validity of revocation of part %d", i)
		}

		err = splitticket.CheckParticipantInTicket(splitTx, ticket,
			session.Amount, session.Fee,
			session.ticketOutputAddress, session.splitOutputAddress,
			session.splitChange, session.myIndex, session.splitInputOutpoints(),
			cfg.ChainParams)
		if err != nil {
			return errors.Wrapf(err, "error checking my participation in "+
				" ticket of part %d", i)
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
		SessionId:         uint32(session.ID),
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
		return errors.Wrap(err, "error decoding funded split")
	}

	err = splitticket.CheckSplit(session.splitTx, session.splitTxUtxoMap,
		session.secretHashes(), session.mainchainHash, session.mainchainHeight,
		cfg.ChainParams)
	if err != nil {
		return err
	}

	err = splitticket.CheckSignedSplit(fundedSplit, session.splitTxUtxoMap,
		cfg.ChainParams)
	if err != nil {
		return err
	}

	if len(resp.SecretNumbers) != len(session.participants) {
		return errors.Errorf("len(secrets) != len(participants)")
	}

	for i, s := range resp.SecretNumbers {
		session.participants[i].secretNb = splitticket.SecretNumber(s)
	}

	session.voterIndex = session.findVoterIndex()
	if session.voterIndex < 0 {
		return errors.Errorf("error finding voter index")
	}

	voter := session.participants[session.voterIndex]

	err = splitticket.CheckSelectedVoter(session.secretNumbers(),
		session.secretHashes(), session.amounts(), session.voteScripts(),
		voter.ticket, session.mainchainHash)
	if err != nil {
		return err
	}

	// TODO: verify if the published ticket transaction (received from the network)
	// actually is for the given voter index. Probably need to alert dcrd to
	// watch for transactions involving all voting addresses and alert on any
	// published that is not for the given ticket

	session.fundedSplitTx = fundedSplit
	session.selectedTicket = voter.ticket
	session.selectedRevocation = voter.revocation

	return nil
}

func (mc *MatcherClient) Close() {
	mc.conn.Close()
}
