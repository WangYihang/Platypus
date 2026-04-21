package agent

import (
	"sync"
	"testing"
)

func TestProcessMapSetAndGet(t *testing.T) {
	pm := NewProcessMap()
	p := &AgentProcess{}
	pm.Set("key1", p)

	got, ok := pm.Get("key1")
	if !ok {
		t.Fatal("expected process to be found")
	}
	if got != p {
		t.Fatal("expected same process pointer")
	}
}

func TestProcessMapDelete(t *testing.T) {
	pm := NewProcessMap()
	pm.Set("key1", &AgentProcess{})
	pm.Delete("key1")

	if _, ok := pm.Get("key1"); ok {
		t.Fatal("expected process to be deleted")
	}
}

func TestProcessMapConcurrentAccess(t *testing.T) {
	pm := NewProcessMap()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(2)
		key := string(rune('a' + i%26))
		go func() {
			defer wg.Done()
			pm.Set(key, &AgentProcess{})
		}()
		go func() {
			defer wg.Done()
			pm.Get(key)
		}()
	}
	wg.Wait()
}
