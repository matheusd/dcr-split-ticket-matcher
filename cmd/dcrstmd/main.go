package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/matheusd/dcr-split-ticket-matcher/pkg"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/daemon"
)

func startDaemon(serverCtx context.Context) error {
	cfg, err := daemon.LoadConfig()
	if err != nil {
		if err == daemon.ErrHelpRequested {
			return nil
		} else if err == daemon.ErrVersionRequested {
			fmt.Printf("Split ticket matcher service daemon version %s\n",
				pkg.Version)
			return nil
		}

		fmt.Println(err)
		os.Exit(1)
	}

	d, err := daemon.NewDaemon(cfg)
	if err != nil {
		panic(err)
	}

	return d.Run(serverCtx)
}

func main() {
	sigs := make(chan os.Signal, 1)

	signal.Notify(sigs)

	serverCtx, cancelFunc := context.WithCancel(context.Background())
	startDaemon(serverCtx)

	for {
		sig := <-sigs
		fmt.Printf("Signal %s received\n", sig)
		cancelFunc()
		time.Sleep(5 * time.Second)
		if sig != syscall.SIGHUP {
			// anything other than SIGHUP is a final signal
			break
		}

		serverCtx, cancelFunc = context.WithCancel(context.Background())
		startDaemon(serverCtx)
	}
}
