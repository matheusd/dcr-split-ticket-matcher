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
			os.Exit(0)
		} else if err == daemon.ErrVersionRequested {
			fmt.Printf("Split ticket matcher service daemon version %s\n",
				pkg.Version)
			os.Exit(0)
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

	s := struct{}{}
	acceptableSignals := map[os.Signal]struct{}{
		syscall.SIGHUP: s, syscall.SIGKILL: s, syscall.SIGQUIT: s,
		syscall.SIGINT: s, syscall.SIGTERM: s, syscall.SIGABRT: s,
	}

	for {
		sig := <-sigs
		if _, has := acceptableSignals[sig]; !has {
			continue
		}

		cancelFunc()
		time.Sleep(100 * time.Millisecond)
		if sig != syscall.SIGHUP {
			// anything other than SIGHUP is a final signal
			break
		}

		serverCtx, cancelFunc = context.WithCancel(context.Background())
		startDaemon(serverCtx)
	}
}
