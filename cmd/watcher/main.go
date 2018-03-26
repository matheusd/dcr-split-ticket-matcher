package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/decred/dcrd/dcrutil"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/buyer"
)

type stdoutListWatcher struct{}

func (w stdoutListWatcher) ListChanged(amounts []dcrutil.Amount) {
	strs := make([]string, len(amounts))
	for i, a := range amounts {
		strs[i] = a.String()
	}
	now := time.Now().Format("2006-01-02 15:04:05 -0700")
	fmt.Printf("%s > Waiting participants: [%s]\n", now, strings.Join(strs, ", "))
}

func main() {

	matcherHost := "localhost:8475"

	mc, err := buyer.ConnectToMatcherService(matcherHost)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	err = mc.WatchWaitingList(ctx, stdoutListWatcher{})
	if err != nil {
		panic(err)
	}

	fmt.Println("Starting to watch waiting list")
	<-ctx.Done()
	fmt.Println("Done waiting. %v", ctx.Err())
}
