package link

import (
	"io"
	"net"
	"testing"
	"time"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// Session wraps a yamux.Session and ties stream-open to writing a
// StreamHeader. Tests drive client/server sessions against an
// in-memory net.Pipe so no real network / WS is involved.

// pairedSessions builds client + server Sessions bridged by net.Pipe.
// Both ends are closed by t.Cleanup.
func pairedSessions(t *testing.T) (*Session, *Session) {
	t.Helper()
	clientConn, serverConn := net.Pipe()

	// yamux.Server and yamux.Client both do an initial handshake that
	// must complete before Open / Accept returns; run them in parallel.
	type result struct {
		s   *Session
		err error
	}
	serverCh := make(chan result, 1)
	go func() {
		s, err := NewServerSession(serverConn)
		serverCh <- result{s, err}
	}()

	clientSess, err := NewClientSession(clientConn)
	if err != nil {
		t.Fatalf("NewClientSession: %v", err)
	}
	serverRes := <-serverCh
	if serverRes.err != nil {
		t.Fatalf("NewServerSession: %v", serverRes.err)
	}

	t.Cleanup(func() {
		clientSess.Close()
		serverRes.s.Close()
	})
	return clientSess, serverRes.s
}

// Happy path: client opens a stream with a ProcessOpenRequest as the
// metadata; server accepts and sees the same type, the same metadata
// bytes, and the same correlation id.
func TestSession_OpenAccept_Roundtrip(t *testing.T) {
	client, server := pairedSessions(t)

	wantMeta := &v2pb.ProcessOpenRequest{Command: "uname", Args: []string{"-a"}}
	metaBytes, err := marshalMeta(wantMeta)
	if err != nil {
		t.Fatalf("marshalMeta: %v", err)
	}

	// Server goroutine: Accept, verify, close.
	done := make(chan error, 1)
	go func() {
		hdr, stream, err := server.Accept()
		if err != nil {
			done <- err
			return
		}
		defer stream.Close()
		if hdr.Type != v2pb.StreamType_STREAM_TYPE_PROCESS_OPEN {
			done <- errTypeMismatch(hdr.Type, v2pb.StreamType_STREAM_TYPE_PROCESS_OPEN)
			return
		}
		if hdr.CorrelationId != "corr-42" {
			done <- errCorrMismatch(hdr.CorrelationId, "corr-42")
			return
		}
		// Metadata round-trip — server unmarshals back to the concrete
		// request and checks the command field.
		var got v2pb.ProcessOpenRequest
		if err := unmarshalMeta(hdr.Metadata, &got); err != nil {
			done <- err
			return
		}
		if got.Command != "uname" {
			done <- errCmdMismatch(got.Command, "uname")
			return
		}
		done <- nil
	}()

	stream, err := client.Open(v2pb.StreamType_STREAM_TYPE_PROCESS_OPEN, metaBytes, "corr-42")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer stream.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("server goroutine: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server goroutine timed out")
	}
}

// Client can write application bytes to the stream after the header;
// server reads them verbatim. Validates the stream is a usable
// io.ReadWriteCloser post-handshake.
func TestSession_StreamCarriesPayload(t *testing.T) {
	client, server := pairedSessions(t)

	done := make(chan error, 1)
	go func() {
		_, stream, err := server.Accept()
		if err != nil {
			done <- err
			return
		}
		defer stream.Close()
		buf := make([]byte, 5)
		if _, err := io.ReadFull(stream, buf); err != nil {
			done <- err
			return
		}
		if string(buf) != "hello" {
			done <- errPayloadMismatch(string(buf), "hello")
			return
		}
		done <- nil
	}()

	stream, err := client.Open(v2pb.StreamType_STREAM_TYPE_EVENT, nil, "")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := stream.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	stream.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("server goroutine: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server goroutine timed out")
	}
}

// After the session closes, Open must fail rather than block.
func TestSession_OpenAfterCloseFails(t *testing.T) {
	client, _ := pairedSessions(t)
	client.Close()
	if _, err := client.Open(v2pb.StreamType_STREAM_TYPE_RPC, nil, ""); err == nil {
		t.Fatal("Open on closed session should error")
	}
}

// Helpers: typed error values so failures point at specific fields
// without the tests growing by a wall of string formatting.

func errTypeMismatch(got, want v2pb.StreamType) error {
	return &assertionErr{msg: "stream type", got: got.String(), want: want.String()}
}
func errCorrMismatch(got, want string) error {
	return &assertionErr{msg: "correlation id", got: got, want: want}
}
func errCmdMismatch(got, want string) error {
	return &assertionErr{msg: "command", got: got, want: want}
}
func errPayloadMismatch(got, want string) error {
	return &assertionErr{msg: "payload", got: got, want: want}
}

type assertionErr struct {
	msg       string
	got, want string
}

func (e *assertionErr) Error() string {
	return e.msg + ": got " + e.got + "; want " + e.want
}
