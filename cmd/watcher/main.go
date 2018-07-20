package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	flags "github.com/btcsuite/go-flags"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/buyer"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
)

type config struct {
	Host     string `long:"host" description:"Address of the matcher service"`
	CertFile string `long:"certfile" description:"Path to certificate file when connecting to custom matchers"`
}

type stdoutListWatcher struct{}

func (w *stdoutListWatcher) WaitingListChanged(queues []matcher.WaitingQueue) {
	if len(queues) == 0 {
		fmt.Println("(queues are empty)")
	}
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

	cfg := &config{}
	parser := flags.NewParser(cfg, flags.Default)
	_, err := parser.Parse()
	if err != nil {
		e, ok := err.(*flags.Error)
		if ok && e.Type == flags.ErrHelp {
			return
		}
		fmt.Printf("Error parsing command line arguments: %v\n", err)
		os.Exit(1)
	}

	if cfg.Host == "" {
		fmt.Println("Specify target host address via --host argument")
		os.Exit(1)
	}

	fmt.Println("Starting to watch waiting list")

	ctx := context.Background()
	err = buyer.WatchMatcherWaitingList(ctx, cfg.Host, cfg.CertFile,
		&stdoutListWatcher{})
	if err != nil {
		panic(err)
	}

	<-ctx.Done()
	fmt.Printf("Done waiting. %v\n", ctx.Err())
}
