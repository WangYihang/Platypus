package core

import (
	"sync"
	"testing"

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
	s.Register("a-1", &link.Session{})
	s.Unregister("a-1")
	if _, ok := s.Get("a-1"); ok {
		t.Fatal("Get after Unregister returned ok=true")
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

	if prev := s.Register("a-1", first); prev != nil {
		t.Fatalf("first Register returned prev=%v; want nil", prev)
	}
	prev := s.Register("a-1", second)
	if prev != first {
		t.Fatalf("second Register returned prev=%v; want first", prev)
	}
	got, _ := s.Get("a-1")
	if got != second {
		t.Fatal("Get after second Register did not return the second session")
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
			s.Register(id, &link.Session{})
			_, _ = s.Get(id)
			if i%4 == 0 {
				s.Unregister(id)
			}
		}()
	}
	wg.Wait()
}
