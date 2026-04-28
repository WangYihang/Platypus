package core

import (
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/link"
)

// AgentLinkService is the server-side registry of active v2 agent
// links: map agent_id → *link.Session. HTTP handlers (terminal WS,
// file API, etc.) look up an agent by id and open yamux streams on
// its Session.

func TestAgentLinkService_RegisterGet(t *testing.T) {
	s := NewAgentLinkService()
	sess := &link.Session{}
	s.Register("a-1", sess)

	got, ok := s.Get("a-1")
	if !ok {
		t.Fatal("Get(a-1) ok = false; want true")
	}
	if got != sess {
		t.Fatalf("Get returned %p; want %p", got, sess)
	}
}

func TestAgentLinkService_GetMissing(t *testing.T) {
	s := NewAgentLinkService()
	if _, ok := s.Get("nope"); ok {
		t.Fatal("Get of unknown id returned ok=true")
	}
}

func TestAgentLinkService_Unregister(t *testing.T) {
	s := NewAgentLinkService()
	sess := &link.Session{}
	s.Register("a-1", sess)
	s.Unregister("a-1", sess)
	if _, ok := s.Get("a-1"); ok {
		t.Fatal("Get after Unregister returned ok=true")
	}
}

// Reconnect-displacement race: the displaced session's defer calls
// Unregister after Register has already replaced the entry with the
// new session. The old code keyed Unregister by agent_id alone, so
// the late call removed the live new-session entry — handlers
// looking up the agent then saw "not connected" even though the
// link was healthy. Fix: Unregister compares the session pointer
// and only deletes if it still matches.
func TestAgentLinkService_UnregisterDisplacedDoesNotRemoveLive(t *testing.T) {
	s := NewAgentLinkService()
	first := &link.Session{}
	second := &link.Session{}

	s.Register("a-1", first)
	if _, prev := s.Register("a-1", second); prev != first {
		t.Fatalf("second Register returned prev=%v; want first", prev)
	}

	// The displaced first session's deferred Unregister fires now.
	// It must NOT remove the live second session.
	s.Unregister("a-1", first)

	got, ok := s.Get("a-1")
	if !ok {
		t.Fatal("Get(a-1) ok=false after stale Unregister; live session was wrongly removed")
	}
	if got != second {
		t.Fatalf("Get returned %p; want %p (the live second session)", got, second)
	}
}

// Register with a duplicate id replaces the earlier session and
// returns the displaced one so the caller can Close() it. Agents
// sometimes reconnect before the server noticed the old session
// died; second login wins.
func TestAgentLinkService_DuplicateRegisterReturnsPrevious(t *testing.T) {
	s := NewAgentLinkService()
	first := &link.Session{}
	second := &link.Session{}

	if _, prev := s.Register("a-1", first); prev != nil {
		t.Fatalf("first Register returned prev=%v; want nil", prev)
	}
	_, prev := s.Register("a-1", second)
	if prev != first {
		t.Fatalf("second Register returned prev=%v; want first", prev)
	}
	got, _ := s.Get("a-1")
	if got != second {
		t.Fatal("Get after second Register did not return the second session")
	}
}

// Register stamps a freshly generated session_id on every call, and
// the same id surfaces through both GetWithSessionID and SessionIDFor.
// This is the link.session_id every log line on this connection
// carries; collisions across reconnects would silently merge log
// streams from unrelated agents on the operator side, so this is a
// regression test more than a feature test.
func TestAgentLinkService_RegisterStampsSessionID(t *testing.T) {
	s := NewAgentLinkService()
	id, _ := s.Register("a-1", &link.Session{})
	if id == "" {
		t.Fatal("Register returned empty session_id")
	}

	_, id2, ok := s.GetWithSessionID("a-1")
	if !ok {
		t.Fatal("GetWithSessionID ok=false after Register")
	}
	if id2 != id {
		t.Fatalf("GetWithSessionID returned %q; want %q", id2, id)
	}
	if got := s.SessionIDFor("a-1"); got != id {
		t.Fatalf("SessionIDFor returned %q; want %q", got, id)
	}

	// Reconnect path: Register again must return a fresh id.
	id3, _ := s.Register("a-1", &link.Session{})
	if id3 == id {
		t.Fatalf("reregister returned identical session_id %q", id3)
	}
}

// All returns a defensive copy so callers iterating it don't
// race the registry itself.
func TestAgentLinkService_AllSnapshot(t *testing.T) {
	s := NewAgentLinkService()
	s.Register("a-1", &link.Session{})
	s.Register("a-2", &link.Session{})

	snap := s.All()
	if len(snap) != 2 {
		t.Fatalf("snapshot len = %d; want 2", len(snap))
	}

	// Mutating the snapshot must not affect the registry.
	delete(snap, "a-1")
	if _, ok := s.Get("a-1"); !ok {
		t.Fatal("mutating snapshot affected registry")
	}
}

// CloseAll is the shutdown hook: every registered session has its
// underlying yamux torn down so accept loops in their hijacked-WS
// handlers unblock and http.Server.Shutdown can complete within the
// 30s grace window. Without this, every SIGTERM with at least one
// connected agent waits the full timeout before the process exits.
func TestAgentLinkService_CloseAllUnblocksAccept(t *testing.T) {
	s := NewAgentLinkService()

	// Build two real paired Sessions per agent so Accept on the
	// "server side" can be awaited; CloseAll on the "client side"
	// (which is what s holds) must propagate teardown.
	mkPair := func() (clientSess, serverSess *link.Session) {
		t.Helper()
		c, srv := net.Pipe()
		ch := make(chan *link.Session, 1)
		go func() {
			ss, err := link.NewServerSession(srv)
			if err != nil {
				t.Errorf("server session: %v", err)
				return
			}
			ch <- ss
		}()
		clientSess, err := link.NewClientSession(c)
		if err != nil {
			t.Fatalf("client session: %v", err)
		}
		return clientSess, <-ch
	}
	c1, p1 := mkPair()
	c2, p2 := mkPair()
	s.Register("a-1", c1)
	s.Register("a-2", c2)

	// Each peer Accept must block until CloseAll fires.
	type acceptResult struct{ err error }
	out := make(chan acceptResult, 2)
	for _, p := range []*link.Session{p1, p2} {
		p := p
		go func() {
			_, _, err := p.Accept()
			out <- acceptResult{err: err}
		}()
	}

	// Sanity: Accept should still be blocked here.
	select {
	case r := <-out:
		t.Fatalf("Accept returned before CloseAll: err=%v", r.err)
	case <-time.After(50 * time.Millisecond):
	}

	s.CloseAll()

	// Both Accepts must unblock with EOF (yamux session shutdown).
	for i := 0; i < 2; i++ {
		select {
		case r := <-out:
			if r.err != nil && !errors.Is(r.err, io.EOF) {
				// Any non-nil err is acceptable (the session was torn
				// down somehow); we're guarding against "Accept stays
				// blocked forever".
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Accept did not unblock within 2s of CloseAll")
		}
	}

	// Registry is emptied so a follow-up shutdown sweep is a no-op.
	if got := s.IDs(); len(got) != 0 {
		t.Fatalf("IDs after CloseAll = %v; want empty", got)
	}
}

// Concurrent Register + Get must not race. The Go race detector
// catches map read/write collisions at runtime; this test exists
// to arm it.
func TestAgentLinkService_ConcurrentSafe(t *testing.T) {
	s := NewAgentLinkService()
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			id := string(rune('A' + (i % 8)))
			sess := &link.Session{}
			s.Register(id, sess)
			_, _ = s.Get(id)
			if i%4 == 0 {
				s.Unregister(id, sess)
			}
		}()
	}
	wg.Wait()
}
