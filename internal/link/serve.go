package link

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/coder/websocket"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// StreamHandler consumes a single accepted yamux stream on the
// server side. It receives the fully-parsed StreamHeader plus the
// live stream; once the handler returns, Serve moves on to the
// next stream. The handler is responsible for closing the stream
// (explicitly or by returning — Serve also defers Close as a
// safety net).
//
// Runs on its own goroutine per stream, so handlers are free to do
// blocking I/O without starving the accept loop.
type StreamHandler func(ctx context.Context, hdr *v2pb.StreamHeader, stream io.ReadWriteCloser)

// Serve is the HTTP handler for the agent-link endpoint. It upgrades
// to WebSocket (requiring the v2 subprotocol), wraps the resulting
// connection in a yamux server Session, and runs an accept loop
// that dispatches each new stream to a goroutine running handler.
//
// Blocks until the session closes (peer disconnect, ctx cancellation,
// or fatal yamux error). Returns nil on a graceful peer-initiated
// close; a non-nil error indicates something went wrong before the
// session could be established, or the accept loop tripped on a
// non-EOF error.
func Serve(ctx context.Context, w http.ResponseWriter, r *http.Request, handler StreamHandler) error {
	if handler == nil {
		return errors.New("link: Serve: handler required")
	}

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{Subprotocol},
	})
	if err != nil {
		return fmt.Errorf("link: ws upgrade: %w", err)
	}
	// CloseNow short-circuits the WS close handshake on the error
	// path; on graceful return below we let the yamux Close unwind
	// the underlying net.Conn which cleanly closes the WS.
	defer func() { _ = c.CloseNow() }()

	nc := websocket.NetConn(context.Background(), c, websocket.MessageBinary)
	sess, err := NewServerSession(nc)
	if err != nil {
		return err
	}
	defer func() { _ = sess.Close() }()

	// Accept loop. Each accepted stream gets its own goroutine so a
	// slow handler can't starve the loop. The loop exits when the
	// peer closes the session (io.EOF from Accept) or ctx cancels.
	acceptCtx, cancelAccept := context.WithCancel(ctx)
	defer cancelAccept()
	go func() {
		<-acceptCtx.Done()
		_ = sess.Close()
	}()

	for {
		hdr, stream, err := sess.Accept()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			// Yamux returns a non-EOF error when the session is
			// being torn down by our own Close call from the ctx
			// goroutine above — treat that the same as EOF.
			select {
			case <-acceptCtx.Done():
				return nil
			default:
			}
			return err
		}
		go func(h *v2pb.StreamHeader, s io.ReadWriteCloser) {
			defer func() { _ = s.Close() }()
			handler(acceptCtx, h, s)
		}(hdr, stream)
	}
}
