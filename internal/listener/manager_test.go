package listener

import (
	"sync"
	"testing"
)

type mockListener struct {
	hash      string
	host      string
	port      uint16
	encrypted bool
	stopped   bool
}

func (m *mockListener) GetHash() string   { return m.hash }
func (m *mockListener) GetHost() string   { return m.host }
func (m *mockListener) GetPort() uint16   { return m.port }
func (m *mockListener) IsEncrypted() bool { return m.encrypted }
func (m *mockListener) FullDesc() string  { return m.hash }
func (m *mockListener) Run()              {}
func (m *mockListener) Stop()             { m.stopped = true }

func TestManagerAddAndGet(t *testing.T) {
	m := NewManager()
	l := &mockListener{hash: "abc123", host: "0.0.0.0", port: 1337}
	m.Add(l)

	got, ok := m.Get("abc123")
	if !ok {
		t.Fatal("expected listener to be found")
	}
	if got.GetHash() != "abc123" {
		t.Fatalf("expected hash abc123, got %s", got.GetHash())
	}
}

func TestManagerRemoveStops(t *testing.T) {
	m := NewManager()
	l := &mockListener{hash: "abc123"}
	m.Add(l)
	m.Remove("abc123")

	if !l.stopped {
		t.Error("expected Stop() to be called on remove")
	}
	if m.Count() != 0 {
		t.Fatalf("expected count 0, got %d", m.Count())
	}
}

func TestManagerFindByHashPrefix(t *testing.T) {
	m := NewManager()
	m.Add(&mockListener{hash: "abc123"})
	m.Add(&mockListener{hash: "def456"})

	if l := m.FindByHashPrefix("abc"); l == nil {
		t.Error("expected to find by prefix")
	}
	if l := m.FindByHashPrefix("xyz"); l != nil {
		t.Error("expected nil for non-matching prefix")
	}
}

func TestManagerAll(t *testing.T) {
	m := NewManager()
	m.Add(&mockListener{hash: "a"})
	m.Add(&mockListener{hash: "b"})
	if len(m.All()) != 2 {
		t.Fatalf("expected 2 listeners, got %d", len(m.All()))
	}
}

func TestManagerConcurrent(t *testing.T) {
	m := NewManager()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m.Add(&mockListener{hash: string(rune('a' + i%26))})
			m.All()
			m.Count()
		}(i)
	}
	wg.Wait()
}
