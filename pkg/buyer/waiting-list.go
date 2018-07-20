package buyer

import (
	"context"
	"crypto/tls"
	"time"

	pb "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/matcherrpc"
	intnet "github.com/matheusd/dcr-split-ticket-matcher/pkg/buyer/internal/net"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type waitingListWatcher interface {
	WaitingListChanged([]matcher.WaitingQueue)
}

// WatchMatcherWaitingList will watch the waiting list queues of the matcher
// given by the arguments, until the context is Done or the matcher sends
// an error.
// Whenever the waiting list changes, the changesChan receives a list with the
// current queues.
func WatchMatcherWaitingList(ctx context.Context, matcherHost string,
	certFile string, watcher waitingListWatcher) error {

	matcherHost, _, err := intnet.DetermineMatcherHost(matcherHost)
	if err != nil {
		return errors.Wrap(err, "error determining matcher host to connect "+
			"to when watching lists")
	}

	var opt grpc.DialOption
	var creds credentials.TransportCredentials

	if certFile != "" {
		creds, err = credentials.NewClientTLSFromFile(certFile, "localhost")
		if err != nil {
			return errors.Wrapf(err, "error creating credentials")
		}
	} else {
		tlsCfg := &tls.Config{
			ServerName: intnet.RemoveHostPort(matcherHost),
		}
		creds = credentials.NewTLS(tlsCfg)
	}
	opt = grpc.WithTransportCredentials(creds)

	dialCtx, cancelDialCtx := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelDialCtx()

	conn, err := grpc.DialContext(dialCtx, matcherHost, opt)
	if err != nil {
		return err
	}

	client := pb.NewSplitTicketMatcherServiceClient(conn)

	req := &pb.WatchWaitingListRequest{SendCurrent: true}
	cli, err := client.WatchWaitingList(ctx, req)
	if err != nil {
		return err
	}

	for {
		resp, err := cli.Recv()
		if err != nil {
			return errors.Wrapf(err, "error while receiving waiting list updates")
		}

		queues := make([]matcher.WaitingQueue, len(resp.Queues))
		for i, q := range resp.Queues {
			queues[i] = matcher.WaitingQueue{
				Name:    q.Name,
				Amounts: uint64sToAmounts(q.Amounts),
			}
		}
		watcher.WaitingListChanged(queues)
	}
}
