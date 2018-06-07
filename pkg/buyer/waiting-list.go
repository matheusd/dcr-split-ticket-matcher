package buyer

import (
	"context"

	pb "github.com/matheusd/dcr-split-ticket-matcher/pkg/api/matcherrpc"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
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

	opt := grpc.WithInsecure()
	if certFile != "" {
		creds, err := credentials.NewClientTLSFromFile(certFile, "localhost")
		if err != nil {
			return err
		}

		opt = grpc.WithTransportCredentials(creds)
	}

	conn, err := grpc.Dial(matcherHost, opt)
	if err != nil {
		return err
	}

	client := pb.NewSplitTicketMatcherServiceClient(conn)

	req := &pb.WatchWaitingListRequest{}
	cli, err := client.WatchWaitingList(ctx, req)
	if err != nil {
		return err
	}

	for {
		resp, err := cli.Recv()
		if err != nil {
			return err
		} else {
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
}
