// Package core — SysInfo cache.
//
// Agents push SysInfoResponse envelopes every ~30 s (see
// internal/agent/sysinfo.go). This in-memory cache holds the most
// recent sample per agent hash so the Topology REST handler can embed
// live CPU / memory / OS details without round-tripping to the agent.
// The cache is best-effort; entries expire when the agent disconnects
// (DeleteAgentClient clears them).
package core

import (
	"sync"
	"time"

	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// SysInfoEntry is a single cached sample plus the observation timestamp.
type SysInfoEntry struct {
	Info       *agentpb.SysInfo
	ReceivedAt time.Time
}

// sysInfoCache is a concurrency-safe map keyed by AgentClient.Hash.
// Writes come from AgentMessageDispatcher on SysInfoResponse; reads
// come from the topology aggregator.
type sysInfoCache struct {
	mu      sync.RWMutex
	byAgent map[string]SysInfoEntry
}

var globalSysInfoCache = &sysInfoCache{byAgent: map[string]SysInfoEntry{}}

// PutSysInfo stores the most recent sample for an agent hash. Nil info
// is a no-op so the caller doesn't have to guard upstream.
func PutSysInfo(agentHash string, info *agentpb.SysInfo) {
	if agentHash == "" || info == nil {
		return
	}
	globalSysInfoCache.mu.Lock()
	globalSysInfoCache.byAgent[agentHash] = SysInfoEntry{
		Info:       info,
		ReceivedAt: time.Now(),
	}
	globalSysInfoCache.mu.Unlock()
}

// GetSysInfo returns the cached SysInfo for an agent hash. The second
// return value is false when there is no entry yet.
func GetSysInfo(agentHash string) (SysInfoEntry, bool) {
	globalSysInfoCache.mu.RLock()
	defer globalSysInfoCache.mu.RUnlock()
	e, ok := globalSysInfoCache.byAgent[agentHash]
	return e, ok
}

// DropSysInfo clears a cache entry — called from DeleteAgentClient so
// the cache doesn't grow unbounded across reconnects.
func DropSysInfo(agentHash string) {
	if agentHash == "" {
		return
	}
	globalSysInfoCache.mu.Lock()
	delete(globalSysInfoCache.byAgent, agentHash)
	globalSysInfoCache.mu.Unlock()
}
