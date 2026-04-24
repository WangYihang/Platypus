package ingress

import (
	"errors"
	"net"
	"sync"
)

// virtualListener adapts a channel of already-accepted net.Conn values
// into a net.Listener, so Go's http.Server (or any other library that
// insists on owning the Accept loop) can be driven by the dispatcher's
// ALPN-routed connections.
//
// Close() is idempotent; the underlying channel is closed exactly once
// and subsequent Accept calls return net.ErrClosed.
type virtualListener struct {
	addr  net.Addr
	conns chan net.Conn

	closeOnce sync.Once
	closed    chan struct{}
}

func newVirtualListener(addr net.Addr, buf int) *virtualListener {
	return &virtualListener{
		addr:   addr,
		conns:  make(chan net.Conn, buf),
		closed: make(chan struct{}),
	}
}

// Accept blocks until the dispatcher delivers a post-TLS connection
// via push, or until the listener is closed. Returns net.ErrClosed on
// close so callers can unwind cleanly.
func (l *virtualListener) Accept() (net.Conn, error) {
	select {
	case c, ok := <-l.conns:
		if !ok {
			return nil, net.ErrClosed
		}
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

// Close signals the listener to stop. Any further push() drops the
// connection.
func (l *virtualListener) Close() error {
	l.closeOnce.Do(func() {
		close(l.closed)
		close(l.conns)
	})
	return nil
}

// Addr reports the address the underlying TLS listener is bound to,
// not a synthetic one, so http.Server log lines and tests using
// Addr().String() still make sense.
func (l *virtualListener) Addr() net.Addr { return l.addr }

// push hands a connection to whoever is currently Accept'ing. Drops
// it with Close() when the listener has already been shut down, so
// the dispatcher's selector never blocks on a stopped sink.
func (l *virtualListener) push(c net.Conn) error {
	select {
	case <-l.closed:
		_ = c.Close()
		return errors.New("ingress: virtual listener closed")
	case l.conns <- c:
		return nil
	}
}
