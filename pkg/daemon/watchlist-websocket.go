package daemon

import (
	"net"
	"net/http"
	"sync"

	"github.com/decred/dcrd/dcrutil"
	"github.com/gorilla/websocket"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/internal/util"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
	logging "github.com/op/go-logging"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type waitlistWebsocketService struct {
	matcher      *matcher.Matcher
	log          *logging.Logger
	upgrader     websocket.Upgrader
	server       *http.Server
	openWatchers *sync.Map
	listener     net.Listener
}

func newWaitlistWebsocketService(bindAddr string, matcher *matcher.Matcher,
	logBackend logging.LeveledBackend) (*waitlistWebsocketService, error) {

	mux := http.NewServeMux()

	ln, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return nil, errors.Wrapf(err, "error binding waiting list websocket"+
			"service to address %s", bindAddr)
	}

	svc := &waitlistWebsocketService{
		matcher:      matcher,
		openWatchers: &sync.Map{},
		log:          logging.MustGetLogger("ws-waiting-list-svc"),
		server:       &http.Server{Addr: bindAddr, Handler: mux},
		listener:     ln,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}

	svc.log.SetBackend(logBackend)
	mux.HandleFunc("/watchWaitingList", svc.watchWaitingList)
	svc.server.RegisterOnShutdown(svc.closeWebsockets)

	return svc, nil
}

func (svc *waitlistWebsocketService) watchWaitingList(w http.ResponseWriter, r *http.Request) {
	c, err := svc.upgrader.Upgrade(w, r, nil)
	if err != nil {
		svc.log.Errorf("Error upgrading websocket connection: %v", err)
		return
	}

	watcher := make(chan []matcher.WaitingQueue)
	svc.matcher.WatchWaitingList(r.Context(), watcher, true)

	srcAddr := r.RemoteAddr // TODO: support reverse proxying
	svc.log.Debugf("New websocket watcher %s", srcAddr)

	type replyqueue struct {
		Name    string           `json:"name"`
		Amounts []dcrutil.Amount `json:"amounts"`
	}

	shutdownChan := make(chan struct{})
	svc.openWatchers.Store(shutdownChan, struct{}{})

	defer c.Close()
	for {
		select {
		case <-r.Context().Done():
			svc.log.Debugf("Done websocket watcher %s", srcAddr)
			svc.openWatchers.Delete(shutdownChan)
			close(shutdownChan)
			return
		case queues := <-watcher:
			reply := make([]replyqueue, len(queues))
			for i, q := range queues {
				reply[i] = replyqueue{encodeQueueName(q.Name), q.Amounts}
			}
			c.WriteJSON(reply)
		case <-shutdownChan:
			close(shutdownChan)
			// no need to delete from openWatchers as we're shuting down anyway
			return
		}
	}
}

func (svc *waitlistWebsocketService) closeWebsockets() {
	svc.log.Noticef("Shutting down outstanding websocket connections")
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

	return svc.server.ServeTLS(util.TCPKeepAliveListener{svc.listener.(*net.TCPListener)},
		certFile, keyFile)
}
