package session

import (
	"net"
	"sync"
	"testing"
	"time"

	oss "github.com/WangYihang/Platypus/internal/utils/os"
)

// mockSession implements Session for testing
type mockSession struct {
	hash          string
	alias         string
	encrypted     bool
	host          string
	port          uint16
	user          string
	os            oss.OperatingSystem
	ts            time.Time
	groupDispatch bool
}

func (m *mockSession) GetHash() string                    { return m.hash }
func (m *mockSession) GetAlias() string                   { return m.alias }
func (m *mockSession) SetAlias(a string)                  { m.alias = a }
func (m *mockSession) IsEncrypted() bool                  { return m.encrypted }
func (m *mockSession) GetHost() string                    { return m.host }
func (m *mockSession) GetPort() uint16                    { return m.port }
func (m *mockSession) GetConn() net.Conn                  { return nil }
func (m *mockSession) GetConnString() string              { return m.host }
func (m *mockSession) GetUsername() string                { return m.user }
func (m *mockSession) GetOS() oss.OperatingSystem         { return m.os }
func (m *mockSession) GetTimeStamp() time.Time            { return m.ts }
func (m *mockSession) GetGroupDispatch() bool             { return m.groupDispatch }
func (m *mockSession) SetGroupDispatch(v bool)            { m.groupDispatch = v }
func (m *mockSession) GetPrompt() string                  { return "» " }
func (m *mockSession) OnelineDesc() string                { return m.hash }
func (m *mockSession) FullDesc() string                   { return m.hash }
func (m *mockSession) AsTable()                           {}
func (m *mockSession) Execute(cmd string) (string, error) { return "", nil }
func (m *mockSession) Close()                             {}

func newMock(hash, alias string) *mockSession {
	return &mockSession{hash: hash, alias: alias, host: "127.0.0.1", port: 1234, ts: time.Now()}
}

func TestManagerAddAndGet(t *testing.T) {
	m := NewManager()
	s := newMock("abc123", "test")
	m.Add(s)

	got, ok := m.Get("abc123")
	if !ok {
		t.Fatal("expected session to be found")
	}
	if got.GetHash() != "abc123" {
		t.Fatalf("expected hash abc123, got %s", got.GetHash())
	}
}

func TestManagerRemove(t *testing.T) {
	m := NewManager()
	m.Add(newMock("abc123", ""))
	m.Remove("abc123")

	if _, ok := m.Get("abc123"); ok {
		t.Fatal("expected session to be removed")
	}
	if m.Count() != 0 {
		t.Fatalf("expected count 0, got %d", m.Count())
	}
}

func TestManagerFindByHashPrefix(t *testing.T) {
	m := NewManager()
	m.Add(newMock("abc123", ""))
	m.Add(newMock("def456", ""))

	if s := m.FindByHashPrefix("abc"); s == nil || s.GetHash() != "abc123" {
		t.Fatal("expected to find abc123 by prefix")
	}
	if s := m.FindByHashPrefix("xyz"); s != nil {
		t.Fatal("expected nil for non-matching prefix")
	}
	if s := m.FindByHashPrefix(""); s != nil {
		t.Fatal("expected nil for empty prefix")
	}
}

func TestManagerFindByAlias(t *testing.T) {
	m := NewManager()
	m.Add(newMock("abc123", "webserver"))
	m.Add(newMock("def456", "database"))

	if s := m.FindByAlias("web"); s == nil || s.GetHash() != "abc123" {
		t.Fatal("expected to find webserver by alias prefix")
	}
	if s := m.FindByAlias("data"); s == nil || s.GetHash() != "def456" {
		t.Fatal("expected to find database by alias prefix")
	}
}

func TestManagerAll(t *testing.T) {
	m := NewManager()
	m.Add(newMock("a", ""))
	m.Add(newMock("b", ""))
	m.Add(newMock("c", ""))

	all := m.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(all))
	}
}

func TestManagerConcurrentAccess(t *testing.T) {
	m := NewManager()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			hash := time.Now().String() + string(rune(i))
			m.Add(newMock(hash, ""))
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.All()
			_ = m.Count()
			_ = m.FindByHashPrefix("x")
		}()
	}

	wg.Wait()
}
