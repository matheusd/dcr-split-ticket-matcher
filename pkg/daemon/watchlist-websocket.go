package daemon

import (
	"net/http"

	"github.com/decred/dcrd/dcrutil"
	"github.com/gorilla/websocket"
	"github.com/matheusd/dcr-split-ticket-matcher/pkg/matcher"
	logging "github.com/op/go-logging"
)

type waitlistWebsocketService struct {
	matcher  *matcher.Matcher
	log      *logging.Logger
	upgrader websocket.Upgrader
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

	defer c.Close()
	for {
		select {
		case <-r.Context().Done():
			svc.log.Debugf("Done websocket watcher %s", srcAddr)
			return
		case queues := <-watcher:
			reply := make([]replyqueue, len(queues))
			for i, q := range queues {
				reply[i] = replyqueue{encodeQueueName(q.Name), q.Amounts}
			}
			c.WriteJSON(reply)
		}
	}
}

func startWaitlistWebsocketServer(bindAddr string, matcher *matcher.Matcher,
	logBackend logging.LeveledBackend) error {
	svc := &waitlistWebsocketService{
		matcher: matcher,
		log:     logging.MustGetLogger("ws-waiting-list-svc"),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}

	svc.log.SetBackend(logBackend)

	mux := http.NewServeMux()
	mux.HandleFunc("/watchWaitingList", svc.watchWaitingList)
	return http.ListenAndServe(bindAddr, mux)
}
