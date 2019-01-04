package buyer

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/decred/dcrd/wire"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/splitticket"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	pb "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/matcherrpc"
	intnet "github.com/matheusd/dcr-split-ticket-matcher/pkg/buyer/internal/net"
)

// utxoMapProvider is the function that will provide the matcher client with
// information about the utxos of a given split transaction.
//
// This function will be passed the split transaction and must return an utxo
// map with entries for all the outpoints in the provided tx or an error.
type utxoMapProvider func(*wire.MsgTx) (splitticket.UtxoMap, error)

// onlineMatcherClient fulfills the buyer's MatcherClient interface by
// connecting to an online matcher service via grpc and relaying all messages to
// it.
type onlineMatcherClient struct {
	client       pb.SplitTicketMatcherServiceClient
	conn         *grpc.ClientConn
	utxoProvider utxoMapProvider
}

// connectToMatcherService tries to connect to the given matcher host and to a
// dcrd daemon, given the provided config options.
func connectToMatcherService(ctx context.Context, matcherHost string,
	certFile string, utxoProvider utxoMapProvider) (MatcherClientConn, error) {

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

	mc := &onlineMatcherClient{
		client:       client,
		conn:         conn,
		utxoProvider: utxoProvider,
	}
	return mc, err
}

func (c *onlineMatcherClient) WatchWaitingList(ctx context.Context, in *pb.WatchWaitingListRequest, opts ...grpc.CallOption) (pb.SplitTicketMatcherService_WatchWaitingListClient, error) {
	return c.client.WatchWaitingList(ctx, in, opts...)
}

func (c *onlineMatcherClient) FindMatches(ctx context.Context, in *pb.FindMatchesRequest, opts ...grpc.CallOption) (*pb.FindMatchesResponse, error) {
	return c.client.FindMatches(ctx, in, opts...)
}

func (c *onlineMatcherClient) GenerateTicket(ctx context.Context, in *pb.GenerateTicketRequest, opts ...grpc.CallOption) (*pb.GenerateTicketResponse, error) {
	return c.client.GenerateTicket(ctx, in, opts...)
}

func (c *onlineMatcherClient) FundTicket(ctx context.Context, in *pb.FundTicketRequest, opts ...grpc.CallOption) (*pb.FundTicketResponse, error) {
	return c.client.FundTicket(ctx, in, opts...)
}

func (c *onlineMatcherClient) FundSplitTx(ctx context.Context, in *pb.FundSplitTxRequest, opts ...grpc.CallOption) (*pb.FundSplitTxResponse, error) {
	return c.client.FundSplitTx(ctx, in, opts...)
}

func (c *onlineMatcherClient) Status(ctx context.Context, in *pb.StatusRequest, opts ...grpc.CallOption) (*pb.StatusResponse, error) {
	return c.client.Status(ctx, in, opts...)
}

func (c *onlineMatcherClient) BuyerError(ctx context.Context, in *pb.BuyerErrorRequest, opts ...grpc.CallOption) (*pb.BuyerErrorResponse, error) {
	return c.client.BuyerError(ctx, in, opts...)
}

func (c *onlineMatcherClient) FetchSpentUtxos(msg *wire.MsgTx) (splitticket.UtxoMap, error) {
	return c.utxoProvider(msg)
}

func (c *onlineMatcherClient) Close() {
	c.conn.Close()
}
