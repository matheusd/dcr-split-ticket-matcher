package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg/buyer"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
)

type stdoutListWatcher struct{}

func (w *stdoutListWatcher) WaitingListChanged(queues []matcher.WaitingQueue) {
	for _, q := range queues {
		strs := make([]string, len(q.Amounts))
		sessName := q.Name
		if len(sessName) > 10 {
			sessName = sessName[:10]
		}
		for i, a := range q.Amounts {
			strs[i] = a.String()
		}
		fmt.Printf("Waiting participants (%s): [%s]\n", sessName, strings.Join(strs, ", "))
	}
}

func main() {

	matcherHost := "testnet-split-tickets.matheusd.com:8475"
	certFile := "samples/testnet-matcher-rpc.cert"

	// FIXME: pass the netconfig

	ctx := context.Background()
	err := buyer.WatchMatcherWaitingList(ctx,
		matcherHost, certFile, &stdoutListWatcher{})
	if err != nil {
		panic(err)
	}

	fmt.Println("Starting to watch waiting list")
	<-ctx.Done()
	fmt.Println("Done waiting. %v", ctx.Err())
}
