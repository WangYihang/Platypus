package link

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// Dial wraps: HTTP(S) dial, WS Upgrade, websocket.NetConn, and
// NewClientSession into a single agent-side constructor. The
// counterpart on the server side (ServeWS in II.9) accepts the
// upgraded connection and builds NewServerSession.

// fakeLinkServer stands up an httptest.Server that, on WS upgrade,
// runs a yamux server session and serves streams off it until the
// client hangs up. Used to exercise Dial without building the real
// AgentLink handler.
//
// It deliberately mirrors the real handler in outliving each
// individual stream. An earlier version returned as soon as it had
// handled one, which tore the whole yamux session down underneath a
// client that was still inside its own Open call — see the loop below.
func fakeLinkServer(t *testing.T, onStream func(hdr *v2pb.StreamHeader, stream interface{ Write(p []byte) (int, error) })) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/agent/link", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"ptps-link-v2"},
		})
		if err != nil {
			t.Errorf("Accept: %v", err)
			return
		}
		nc := websocket.NetConn(context.Background(), c, websocket.MessageBinary)
		defer nc.Close()

		sess, err := NewServerSession(nc)
		if err != nil {
			t.Errorf("NewServerSession: %v", err)
			return
		}
		defer sess.Close()

		// Serve until the client closes the session, rather than
		// tearing everything down after the first stream.
		//
		// Session.Open writes the StreamHeader through yamux, and
		// yamux's Stream.Write does not return until the send loop
		// reports back on that frame. The bytes reach the peer before
		// that report does, so a server that accepted the header,
		// finished with the stream and closed the session could win
		// the race and leave Open returning ErrSessionShutdown for a
		// write whose bytes had in fact been delivered — the flake
		// read `Open: link: write header: ... session shutdown` and
		// hit roughly 1.5% of runs under load. Nothing was wrong on
		// the wire, and nothing was wrong in Session.Open; the
		// harness simply hung up sooner than any real peer would.
		for {
			hdr, stream, err := sess.Accept()
			if err != nil {
				return
			}
			if onStream != nil {
				onStream(hdr, stream)
			}
			stream.Close()
		}
	})
	return httptest.NewServer(mux)
}

// Happy path: Dial → Open → server observes the stream type and the
// correlation id the client supplied.
func TestDial_OpenRoundtrip(t *testing.T) {
	gotType := make(chan v2pb.StreamType, 1)
	gotCorr := make(chan string, 1)
	srv := fakeLinkServer(t, func(hdr *v2pb.StreamHeader, stream interface{ Write(p []byte) (int, error) }) {
		gotType <- hdr.Type
		gotCorr <- hdr.CorrelationId
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	wsURL := strings.Replace(srv.URL, "http://", "ws://", 1) + "/agent/link"
	sess, err := Dial(ctx, DialOptions{URL: wsURL})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer sess.Close()

	stream, err := sess.Open(v2pb.StreamType_STREAM_TYPE_RPC, nil, "corr-dial-1")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer stream.Close()

	select {
	case got := <-gotType:
		if got != v2pb.StreamType_STREAM_TYPE_RPC {
			t.Fatalf("server saw type %v; want STREAM_TYPE_RPC", got)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for server to observe stream type")
	}
	select {
	case got := <-gotCorr:
		if got != "corr-dial-1" {
			t.Fatalf("server saw corr %q; want corr-dial-1", got)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for server to observe correlation id")
	}
}

// Dial against a non-WS URL returns a non-nil error rather than a
// zombie Session.
func TestDial_ServerNotWebSocket(t *testing.T) {
	// Plain-HTTP handler that doesn't do the Upgrade handshake.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := Dial(ctx, DialOptions{URL: srv.URL})
	if err == nil {
		t.Fatal("Dial against non-WS server should fail")
	}
}

// Empty URL is a programming error, not a network error — surface
// it before attempting any I/O.
func TestDial_RequiresURL(t *testing.T) {
	if _, err := Dial(context.Background(), DialOptions{}); err == nil {
		t.Fatal("Dial with empty URL should fail")
	}
}
