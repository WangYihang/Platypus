package agent

import (
	"net"
	"sync"
	"testing"
)

func TestConnMapSetAndGet(t *testing.T) {
	cm := NewConnMap()
	var conn net.Conn // nil is fine for testing
	cm.Set("token1", &conn)

	got, ok := cm.Get("token1")
	if !ok {
		t.Fatal("expected conn to be found")
	}
	if got != &conn {
		t.Fatal("expected same conn pointer")
	}
}

func TestConnMapGetAndDelete(t *testing.T) {
	cm := NewConnMap()
	var conn net.Conn
	cm.Set("token1", &conn)

	got, ok := cm.GetAndDelete("token1")
	if !ok {
		t.Fatal("expected conn to be found")
	}
	if got != &conn {
		t.Fatal("expected same conn pointer")
	}

	// Should be gone now
	if _, ok := cm.Get("token1"); ok {
		t.Fatal("expected conn to be deleted after GetAndDelete")
	}
}

func TestConnMapConcurrentAccess(t *testing.T) {
	cm := NewConnMap()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(2)
		key := string(rune('a' + i%26))
		go func() {
			defer wg.Done()
			var c net.Conn
			cm.Set(key, &c)
		}()
		go func() {
			defer wg.Done()
			cm.GetAndDelete(key)
		}()
	}
	wg.Wait()
}
