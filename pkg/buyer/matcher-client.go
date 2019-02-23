package buyer

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"google.golang.org/grpc/keepalive"

	"github.com/pkg/errors"

	intnet "github.com/matheusd/dcr-split-ticket-matcher/pkg/buyer/internal/net"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/version"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	pb "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/matcherrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// UtxoMapProvider is the function that will provide the matcher client with
// information about the utxos of a given split transaction.
//
// This function will be passed the split transaction and must return an utxo
// map with entries for all the outpoints in the provided tx or an error.
type UtxoMapProvider func(*wire.MsgTx) (splitticket.UtxoMap, error)

// MatcherClient is handles all requests and checks done by the buyer when
// interacting to a remote matcher server.
type MatcherClient struct {
	client       pb.SplitTicketMatcherServiceClient
	conn         *grpc.ClientConn
	utxoProvider UtxoMapProvider
}

// ConnectToMatcherService tries to connect to the given matcher host and to a
// dcrd daemon, given the provided config options.
func ConnectToMatcherService(ctx context.Context, matcherHost string,
	certFile string, utxoProvider UtxoMapProvider) (*MatcherClient, error) {

	rep := reporterFromContext(ctx)

	matcherHost, isSrv, err := intnet.DetermineMatcherHost(matcherHost)
	if err != nil {
		rep.reportSrvLookupError(err)
	}
	if isSrv {
		rep.reportSrvRecordFound(matcherHost)
	}

	var opt grpc.DialOption
	var creds credentials.TransportCredentials

	if certFile != "" {
		creds, err = credentials.NewClientTLSFromFile(certFile, "localhost")
		if err != nil {
			return nil, errors.Wrapf(err, "error creating credentials")
		}
	} else {
		tlsCfg := &tls.Config{
			ServerName: intnet.RemoveHostPort(matcherHost),
		}
		creds = credentials.NewTLS(tlsCfg)
	}
	opt = grpc.WithTransportCredentials(creds)
	optKeepAlive := grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                5 * time.Minute,
		Timeout:             20 * time.Second,
		PermitWithoutStream: true,
	})

	conn, err := grpc.Dial(matcherHost, opt, optKeepAlive)
	if err != nil {
		return nil, errors.Wrapf(err, "error connecting to matcher host")
	}

	client := pb.NewSplitTicketMatcherServiceClient(conn)

	mc := &MatcherClient{
		client:       client,
		conn:         conn,
		utxoProvider: utxoProvider,
	}
	return mc, err
}

func (mc *MatcherClient) status(ctx context.Context) (*pb.StatusResponse, error) {
	req := &pb.StatusRequest{}
	return mc.client.Status(ctx, req)
}

func (mc *MatcherClient) participate(ctx context.Context, maxAmount dcrutil.Amount,
	sessionName string, voteAddress, poolAddress string, poolFeeRate float64,
	chainParams *chaincfg.Params) (*Session, error) {
	req := &pb.FindMatchesRequest{
		Amount:          uint64(maxAmount),
		SessionName:     sessionName,
		ProtocolVersion: version.ProtocolVersion,
		VoteAddress:     voteAddress,
		PoolAddress:     poolAddress,
	}

	resp, err := mc.client.FindMatches(ctx, req)
	if err != nil {
		return nil, err
	}

	mainchainHash, err := chainhash.NewHash(resp.MainchainHash)
	if err != nil {
		return nil, err
	}

	sess := &Session{
		ID:              matcher.ParticipantID(resp.SessionId),
		Amount:          dcrutil.Amount(resp.Amount),
		Fee:             dcrutil.Amount(resp.Fee),
		PoolFee:         dcrutil.Amount(resp.PoolFee),
		TicketPrice:     dcrutil.Amount(resp.TicketPrice),
		mainchainHash:   mainchainHash,
		mainchainHeight: resp.MainchainHeight,
		nbParticipants:  resp.NbParticipants,
		sessionToken:    resp.SessionToken,
	}

	err = splitticket.CheckParticipantSessionPoolFee(int(sess.nbParticipants),
		sess.TicketPrice, sess.Amount, sess.PoolFee, sess.Fee,
		int(sess.mainchainHeight), poolFeeRate, chainParams)
	if err != nil {
		return nil, errors.Wrap(err, "matcher requested wrong pool fee amount")
	}

	// TODO: check mainchainHash, mainchainHeight and partFee for problems

	return sess, nil
}

func (mc *MatcherClient) generateTicket(ctx context.Context, session *Session, cfg *Config) error {

	rep := reporterFromContext(ctx)

	voteAddr, err := dcrutil.DecodeAddress(cfg.VoteAddress)
	if err != nil {
		return err
	}

	poolAddr, err := dcrutil.DecodeAddress(cfg.PoolAddress)
	if err != nil {
		return err
	}

	var splitTxChange *pb.TxOut
	if session.splitChange != nil {
		splitTxChange = &pb.TxOut{
			Script: session.splitChange.PkScript,
			Value:  uint64(session.splitChange.Value),
		}
	}

	session.secretNb = splitticket.SecretNumber(matcher.MustRandUint64())
	session.secretNbHash = session.secretNb.Hash(session.mainchainHash)
	session.voteAddress = voteAddr
	session.poolAddress = poolAddr

	req := &pb.GenerateTicketRequest{
		SessionId:         uint32(session.ID),
		SplitTxChange:     splitTxChange,
		CommitmentAddress: session.ticketOutputAddress.String(),
		SplitTxAddress:    session.splitOutputAddress.String(),
		SecretnbHash:      session.secretNbHash[:],
		SessionToken:      session.sessionToken,
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
	rep.reportStage(ctx, StageFetchingUTXOs, session, cfg)
	utxoMap, err := mc.utxoProvider(session.splitTx)
	if err != nil {
		return err
	}
	session.splitTxUtxoMap = utxoMap

	session.participants = make([]buyerSessionParticipant, len(resp.Participants))
	for i, p := range resp.Participants {
		var voteAddresses []dcrutil.Address

		_, voteAddresses, _, err = txscript.ExtractPkScriptAddrs(
			txscript.DefaultScriptVersion, p.VotePkScript, cfg.ChainParams)
		if err != nil {
			return errors.Wrapf(err, "error decoding vote pkscript of"+
				"participant %d", i)
		}
		if len(voteAddresses) != 1 {
			return errors.Errorf("wrong number of vote addresses (%d) in "+
				"vote pkscript of participant %d", len(voteAddresses), i)
		}

		session.participants[i] = buyerSessionParticipant{
			amount:       dcrutil.Amount(p.Amount),
			poolPkScript: p.PoolPkScript,
			votePkScript: p.VotePkScript,
			voteAddress:  voteAddresses[0],
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

	err = splitticket.CheckSplitLotteryCommitment(session.splitTx,
		session.secretHashes(), session.amounts(), session.voteAddresses(),
		session.mainchainHash)
	if err != nil {
		return errors.Wrapf(err, "error checking lottery commitment in split")
	}

	// ensure the ticket template is valid
	err = splitticket.CheckTicket(session.splitTx, session.ticketTemplate,
		session.TicketPrice, session.Fee, session.amounts(),
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

	// ensure pool fee is not higher than expected
	err = splitticket.CheckTicketPoolFeeRate(session.splitTx,
		session.ticketTemplate, cfg.PoolFeeRate, session.mainchainHeight,
		cfg.ChainParams)
	if err != nil {
		return errors.Wrapf(err, "error checking pool fee rate in ticket")
	}

	return nil
}

func (mc *MatcherClient) fundTicket(ctx context.Context, session *Session, cfg *Config) error {

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
		SessionToken:        session.sessionToken,
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
			session.Fee, partsAmounts, session.mainchainHeight,
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

func (mc *MatcherClient) fundSplitTx(ctx context.Context, session *Session, cfg *Config) error {

	splitTxSigs := make([][]byte, len(session.splitInputs))
	for i, in := range session.splitInputs {
		splitTxSigs[i] = in.SignatureScript
	}

	req := &pb.FundSplitTxRequest{
		SessionId:         uint32(session.ID),
		SplitTxScriptsigs: splitTxSigs,
		Secretnb:          uint64(session.secretNb),
		SessionToken:      session.sessionToken,
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

	selCoin, selIndex := splitticket.CalcLotteryResult(session.secretNumbers(),
		session.amounts(), session.mainchainHash)
	session.voterIndex = selIndex
	session.selectedCoin = selCoin
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

	session.fundedSplitTx = fundedSplit
	session.selectedTicket = voter.ticket
	session.selectedRevocation = voter.revocation

	return nil
}

// sendErrorReport sends the given error to the matcher. It ignores all errors,
// given that any error triggered here (eg: connection error, etc) is
// unreportable anyway or may have been caused by the original error.
func (mc *MatcherClient) sendErrorReport(sessionID matcher.ParticipantID, buyerErr error) {
	req := &pb.BuyerErrorRequest{
		ErrorMsg:  buyerErr.Error(),
		SessionId: uint32(sessionID),
	}

	// use a separate context with timeout so that if the original session
	// context errored, the BuyerError() call won't fail.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	mc.client.BuyerError(ctx, req)
}

func (mc *MatcherClient) close() {
	mc.conn.Close()
}
