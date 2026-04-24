package core

import (
	"sync"

	"github.com/WangYihang/Platypus/internal/link"
)

// AgentLinkService is the process-wide registry of live v2 agent
// links, keyed by agent_id. Handlers that need to open a yamux
// stream against a specific agent (terminal WS, file API, exec RPC,
// etc.) look up the Session here instead of hunting through some
// global map.
//
// All mutations are serialised by a single RWMutex; reads go
// through RLock so a busy "list agents" handler doesn't starve
// links being registered.
type AgentLinkService struct {
	mu    sync.RWMutex
	links map[string]*link.Session
}

// NewAgentLinkService returns an empty registry.
func NewAgentLinkService() *AgentLinkService {
	return &AgentLinkService{links: make(map[string]*link.Session)}
}

// Register installs sess under agentID. If an entry already existed
// for that id (the agent reconnected before the old session died)
// the displaced *Session is returned so the caller can Close() it;
// otherwise nil.
func (s *AgentLinkService) Register(agentID string, sess *link.Session) *link.Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.links[agentID]
	s.links[agentID] = sess
	return prev
}

// Unregister removes agentID. Safe to call on an id that isn't in
// the registry. Does NOT Close the session — the caller owns its
// lifecycle.
func (s *AgentLinkService) Unregister(agentID string) {
	s.mu.Lock()
	delete(s.links, agentID)
	s.mu.Unlock()
}

// Get looks up the session for agentID. The bool return
// distinguishes "no agent with that id" from "registered agent but
// nil session" (which should never happen, but defend).
func (s *AgentLinkService) Get(agentID string) (*link.Session, bool) {
	s.mu.RLock()
	sess, ok := s.links[agentID]
	s.mu.RUnlock()
	return sess, ok
}

// All returns a defensive copy of the registry. Callers iterate
// the returned map freely; mutating it doesn't affect the
// registry.
func (s *AgentLinkService) All() map[string]*link.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]*link.Session, len(s.links))
	for k, v := range s.links {
		out[k] = v
	}
	return out
}

// IDs returns a snapshot of every agent_id with a live link. Cheap
// alternative to All() when callers only need the set of identifiers
// — typically to build a `map[string]bool` for SQL-result intersection
// (the "is this row's agent actually live right now?" check the
// sessions handler uses to filter audit-tail rows out of the live
// view).
func (s *AgentLinkService) IDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.links))
	for k := range s.links {
		out = append(out, k)
	}
	return out
}
