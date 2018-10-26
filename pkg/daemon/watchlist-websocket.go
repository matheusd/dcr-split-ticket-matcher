package daemon

import (
	"net"
	"net/http"
	"sync"

	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/slog"
	"github.com/gorilla/websocket"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type waitlistWebsocketService struct {
	matcher      *matcher.Matcher
	log          slog.Logger
	upgrader     websocket.Upgrader
	server       *http.Server
	openWatchers *sync.Map
	listener     net.Listener
}

func newWaitlistWebsocketService(bindAddr string, matcher *matcher.Matcher,
	log slog.Logger) (*waitlistWebsocketService, error) {

	mux := http.NewServeMux()

	ln, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return nil, errors.Wrapf(err, "error binding waiting list websocket"+
			"service to address %s", bindAddr)
	}

	svc := &waitlistWebsocketService{
		matcher:      matcher,
		openWatchers: &sync.Map{},
		log:          log,
		server:       &http.Server{Addr: bindAddr, Handler: mux},
		listener:     ln,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}

	mux.HandleFunc("/", svc.index)
	mux.HandleFunc("/watchWaitingList", svc.watchWaitingList)
	svc.server.RegisterOnShutdown(svc.closeWebsockets)

	return svc, nil
}

func (svc *waitlistWebsocketService) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("waitlist watcher service\n"))
}

func (svc *waitlistWebsocketService) watchWaitingList(w http.ResponseWriter, r *http.Request) {
	c, err := svc.upgrader.Upgrade(w, r, nil)
	if err != nil {
		svc.log.Errorf("Error upgrading websocket connection: %v", err)
		return
	}

	srcAddr := r.RemoteAddr // TODO: support reverse proxying
	svc.log.Debugf("New websocket watcher %s", srcAddr)

	ctx := matcher.WithOriginalSrc(r.Context(), "[wss]"+srcAddr)

	watcher := make(chan []matcher.WaitingQueue)
	svc.matcher.WatchWaitingList(ctx, watcher, true)

	type replyqueue struct {
		Name    string           `json:"name"`
		Amounts []dcrutil.Amount `json:"amounts"`
	}

	shutdownChan := make(chan struct{})
	svc.openWatchers.Store(shutdownChan, struct{}{})

	readChan := make(chan error)
	go func() {
		var err error
		for err == nil {
			_, _, err = c.ReadMessage()
		}
		readChan <- err
	}()

	defer c.Close()
	for {
		select {
		case err := <-readChan:
			if err != nil {
				svc.log.Debugf("Websocket reader returned error %s", err)
				svc.openWatchers.Delete(shutdownChan)
				return
			}
		case <-r.Context().Done():
			svc.log.Debugf("Done websocket watcher %s", srcAddr)
			svc.openWatchers.Delete(shutdownChan)
			return
		case queues := <-watcher:
			reply := make([]replyqueue, len(queues))
			for i, q := range queues {
				reply[i] = replyqueue{encodeQueueName(q.Name), q.Amounts}
			}
			c.WriteJSON(reply)
		case <-shutdownChan:
			// no need to delete from openWatchers as we're shuting down anyway
			return
		}
	}
}

func (svc *waitlistWebsocketService) closeWebsockets() {
	svc.log.Infof("Shutting down outstanding websocket connections")
	svc.openWatchers.Range(func(key, value interface{}) bool {
		if shutdownChan, is := key.(chan struct{}); is {
			go func(c chan struct{}) { c <- struct{}{} }(shutdownChan)
		}
		return true
	})
}

func (svc *waitlistWebsocketService) run(serverCtx context.Context,
	certFile, keyFile string) error {

	go func() {
		<-serverCtx.Done()
		svc.server.Shutdown(context.Background())
		svc.listener.Close()
	}()

	return svc.server.ServeTLS(svc.listener, certFile, keyFile)
}
