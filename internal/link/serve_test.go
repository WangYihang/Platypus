package link

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// Serve is the server-side entrypoint: an HTTP handler that accepts
// a WS Upgrade, wraps the connection in a yamux server Session, and
// routes every accepted stream to the user-supplied StreamHandler.

// Basic happy path: client Dials + Opens; server's StreamHandler
// sees the stream, reads a byte off it, confirms it's the one the
// client wrote.
func TestServe_DispatchesStreams(t *testing.T) {
	gotHdr := make(chan *v2pb.StreamHeader, 1)
	gotPayload := make(chan string, 1)

	handler := func(_ context.Context, hdr *v2pb.StreamHeader, stream io.ReadWriteCloser) {
		gotHdr <- hdr
		buf := make([]byte, 3)
		if _, err := io.ReadFull(stream, buf); err != nil {
			t.Errorf("handler ReadFull: %v", err)
			return
		}
		gotPayload <- string(buf)
		stream.Close()
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := Serve(r.Context(), w, r, handler); err != nil {
			t.Logf("Serve returned: %v", err)
		}
	}))
	defer srv.Close()

	wsURL := strings.Replace(srv.URL, "http://", "ws://", 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sess, err := Dial(ctx, DialOptions{URL: wsURL})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer sess.Close()

	stream, err := sess.Open(v2pb.StreamType_STREAM_TYPE_RPC, nil, "corr-serve-1")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := stream.Write([]byte("abc")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case hdr := <-gotHdr:
		if hdr.CorrelationId != "corr-serve-1" {
			t.Fatalf("CorrelationId = %q; want corr-serve-1", hdr.CorrelationId)
		}
		if hdr.Type != v2pb.StreamType_STREAM_TYPE_RPC {
			t.Fatalf("Type = %v; want RPC", hdr.Type)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for server handler to see header")
	}
	select {
	case got := <-gotPayload:
		if got != "abc" {
			t.Fatalf("payload = %q; want abc", got)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for server handler to read payload")
	}
}

// Concurrent streams: open N streams on the same session, each gets
// its own handler goroutine. Verifies the accept loop isn't
// serialising handler execution.
func TestServe_HandlesConcurrentStreams(t *testing.T) {
	const N = 8
	var mu sync.Mutex
	var seen []string

	handler := func(_ context.Context, hdr *v2pb.StreamHeader, stream io.ReadWriteCloser) {
		defer stream.Close()
		mu.Lock()
		seen = append(seen, hdr.CorrelationId)
		mu.Unlock()
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = Serve(r.Context(), w, r, handler)
	}))
	defer srv.Close()

	wsURL := strings.Replace(srv.URL, "http://", "ws://", 1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sess, err := Dial(ctx, DialOptions{URL: wsURL})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer sess.Close()

	for i := 0; i < N; i++ {
		corr := "corr-" + string(rune('A'+i))
		stream, err := sess.Open(v2pb.StreamType_STREAM_TYPE_EVENT, nil, corr)
		if err != nil {
			t.Fatalf("Open %d: %v", i, err)
		}
		stream.Close()
	}

	// Poll briefly until every handler goroutine has recorded its
	// correlation id. 1s budget is plenty for in-memory yamux.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(seen)
		mu.Unlock()
		if n == N {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("only %d/%d streams seen by handler: %v", len(seen), N, seen)
}

// If the client never opens a stream and closes the session, Serve
// returns cleanly without the test hanging.
func TestServe_ReturnsOnCleanClose(t *testing.T) {
	handlerCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = Serve(r.Context(), w, r, func(context.Context, *v2pb.StreamHeader, io.ReadWriteCloser) {
			handlerCalled = true
		})
	}))
	defer srv.Close()

	wsURL := strings.Replace(srv.URL, "http://", "ws://", 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	sess, err := Dial(ctx, DialOptions{URL: wsURL})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	sess.Close()
	// Give the server goroutine a moment to unwind; the test
	// passing means Serve returned.
	time.Sleep(50 * time.Millisecond)
	if handlerCalled {
		t.Fatal("handler should not have been called")
	}
}
