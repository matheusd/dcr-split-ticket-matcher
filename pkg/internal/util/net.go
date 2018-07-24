package util

import (
	"net"
	"time"
)

// TCPKeepAliveListener is a listener that sets keepalive options on a
// connection after being accepted.
type TCPKeepAliveListener struct {
	*net.TCPListener
}

// Accept fulfills the listener interface
func (ln TCPKeepAliveListener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}
