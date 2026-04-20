package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// startNotifyServer spins up a test WS server at /notify that sends each
// frame in `frames` to every client that connects, then waits for ctx done.
func startNotifyServer(t *testing.T, frames []string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("Accept: %v", err)
			return
		}
		defer c.Close(websocket.StatusNormalClosure, "")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		for _, f := range frames {
			if err := c.Write(ctx, websocket.MessageText, []byte(f)); err != nil {
				return
			}
		}
		// Hold the connection open briefly so the client has time to read.
		<-ctx.Done()
	})
	return httptest.NewServer(mux)
}

func TestNotifier_ReceivesAndDispatchesEvent(t *testing.T) {
	frames := []string{
		`{"Type":0,"Data":{"Client":{"hash":"abc","host":"1.2.3.4"},"ServerHash":"srv1"}}`,
	}
	srv := startNotifyServer(t, frames)
	defer srv.Close()

	var mu sync.Mutex
	var got []Event
	handler := func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, e)
	}

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	n := NewNotifier(wsURL, "", handler)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := n.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer n.Stop()

	// Wait for at least one event
	deadline := time.After(800 * time.Millisecond)
	for {
		mu.Lock()
		count := len(got)
		mu.Unlock()
		if count >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("did not receive event before deadline")
		case <-time.After(20 * time.Millisecond):
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if got[0].Type != EventClientConnected {
		t.Errorf("event Type = %d, want %d", got[0].Type, EventClientConnected)
	}
	var data struct {
		Client     map[string]any `json:"Client"`
		ServerHash string         `json:"ServerHash"`
	}
	if err := json.Unmarshal(got[0].Data, &data); err != nil {
		t.Fatalf("unmarshal Data: %v", err)
	}
	if data.ServerHash != "srv1" {
		t.Errorf("ServerHash = %q", data.ServerHash)
	}
	if data.Client["hash"] != "abc" {
		t.Errorf("Client.hash = %v", data.Client["hash"])
	}
}

func TestNotifier_ParsesAllKnownEventTypes(t *testing.T) {
	cases := []struct {
		raw  string
		want EventType
	}{
		{`{"Type":0,"Data":{}}`, EventClientConnected},
		{`{"Type":1,"Data":{}}`, EventClientDuplicated},
		{`{"Type":2,"Data":{}}`, EventServerDuplicated},
		{`{"Type":3,"Data":{}}`, EventCompiling},
		{`{"Type":4,"Data":{}}`, EventCompressing},
		{`{"Type":5,"Data":{}}`, EventUploading},
	}
	frames := make([]string, len(cases))
	for i, c := range cases {
		frames[i] = c.raw
	}

	srv := startNotifyServer(t, frames)
	defer srv.Close()

	var mu sync.Mutex
	var got []Event
	handler := func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, e)
	}

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	n := NewNotifier(wsURL, "", handler)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := n.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	deadline := time.After(800 * time.Millisecond)
	for {
		mu.Lock()
		count := len(got)
		mu.Unlock()
		if count >= len(cases) {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("got %d events, want %d", count, len(cases))
		case <-time.After(20 * time.Millisecond):
		}
	}

	mu.Lock()
	defer mu.Unlock()
	for i, c := range cases {
		if got[i].Type != c.want {
			t.Errorf("event %d Type = %d, want %d", i, got[i].Type, c.want)
		}
	}
}

func TestNotifier_StopCleanlyExitsGoroutine(t *testing.T) {
	srv := startNotifyServer(t, nil)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	n := NewNotifier(wsURL, "", func(Event) {})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := n.Start(ctx); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		n.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return within 2s")
	}
}

func TestNotifier_StartFailsOnUnreachableURL(t *testing.T) {
	n := NewNotifier("ws://127.0.0.1:1", "", func(Event) {})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := n.Start(ctx)
	if err == nil {
		t.Fatal("expected dial error")
	}
}

func TestNotifier_IgnoresUnknownEventTypes(t *testing.T) {
	frames := []string{
		`{"Type":99,"Data":{}}`,
		`{"Type":0,"Data":{}}`,
	}
	srv := startNotifyServer(t, frames)
	defer srv.Close()

	var mu sync.Mutex
	var got []Event
	handler := func(e Event) {
		mu.Lock()
		got = append(got, e)
		mu.Unlock()
	}

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	n := NewNotifier(wsURL, "", handler)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := n.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	deadline := time.After(600 * time.Millisecond)
	for {
		mu.Lock()
		count := len(got)
		mu.Unlock()
		if count >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("got %d events", count)
		case <-time.After(20 * time.Millisecond):
		}
	}

	mu.Lock()
	defer mu.Unlock()
	// Unknown type should still be delivered (raw int preserved) so the
	// frontend can decide how to treat it; we just don't strongly type it.
	if got[0].Type != EventType(99) {
		t.Errorf("got[0].Type = %d, want 99", got[0].Type)
	}
}
