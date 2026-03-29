package agent

import (
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

// TermiteProcess represents a PTY-attached process on the agent.
type TermiteProcess struct {
	Ptmx       *os.File
	WindowSize *pty.Winsize
	Process    *exec.Cmd
}

// ProcessMap is a thread-safe map of processes keyed by token.
type ProcessMap struct {
	mu sync.RWMutex
	m  map[string]*TermiteProcess
}

// NewProcessMap creates a new empty ProcessMap.
func NewProcessMap() *ProcessMap {
	return &ProcessMap{m: make(map[string]*TermiteProcess)}
}

func (pm *ProcessMap) Get(key string) (*TermiteProcess, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	p, ok := pm.m[key]
	return p, ok
}

func (pm *ProcessMap) Set(key string, p *TermiteProcess) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.m[key] = p
}

func (pm *ProcessMap) Delete(key string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.m, key)
}
