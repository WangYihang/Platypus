package core

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/WangYihang/Platypus/internal/link"
)

// AgentLinkService is the process-wide registry of live v2 agent
// links, keyed by agent_id. Handlers that need to open a yamux
// stream against a specific agent (terminal WS, file API, exec RPC,
// etc.) look up the Session here instead of hunting through some
// global map.
//
// Each registered link also carries a server-generated session_id
// that stays stable for the lifetime of the underlying yamux session.
// HTTP handlers and RPC plumbing thread this id through into the
// StreamHeader so every log line on both sides of the wire can be
// grouped by `link_session_id`.
//
// All mutations are serialised by a single RWMutex; reads go
// through RLock so a busy "list agents" handler doesn't starve
// links being registered.
type AgentLinkService struct {
	mu    sync.RWMutex
	links map[string]*linkRecord
}

// linkRecord bundles a live agent Session with its server-generated
// session_id and connect timestamp. Stored by value through a
// pointer so concurrent readers and a single writer can share it
// without copying.
type linkRecord struct {
	sess        *link.Session
	sessionID   string
	connectedAt time.Time
}

// NewAgentLinkService returns an empty registry.
func NewAgentLinkService() *AgentLinkService {
	return &AgentLinkService{links: make(map[string]*linkRecord)}
}

// Register installs sess under agentID and returns the freshly
// generated session_id together with any prior *Session that the
// caller is responsible for Close()-ing. Reconnect-before-old-died
// races resolve to "second login wins"; the displaced session is
// returned (non-nil) so the caller can tear it down.
func (s *AgentLinkService) Register(agentID string, sess *link.Session) (sessionID string, displaced *link.Session) {
	rec := &linkRecord{
		sess:        sess,
		sessionID:   newLinkSessionID(),
		connectedAt: time.Now().UTC(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if prev := s.links[agentID]; prev != nil {
		displaced = prev.sess
	}
	s.links[agentID] = rec
	return rec.sessionID, displaced
}

// Unregister removes agentID, but only if the currently registered
// session is `sess`. Compare-and-delete avoids a reconnect-displacement
// race: when a second Register replaces the entry and the displaced
// session's deferred Unregister fires later, we must not remove the
// new live entry. Safe to call on an id that isn't in the registry.
// Does NOT Close the session — the caller owns its lifecycle.
func (s *AgentLinkService) Unregister(agentID string, sess *link.Session) {
	s.mu.Lock()
	if rec, ok := s.links[agentID]; ok && rec.sess == sess {
		delete(s.links, agentID)
	}
	s.mu.Unlock()
}

// Get looks up the session for agentID. The bool return
// distinguishes "no agent with that id" from "registered agent but
// nil session" (which should never happen, but defend).
func (s *AgentLinkService) Get(agentID string) (*link.Session, bool) {
	s.mu.RLock()
	rec, ok := s.links[agentID]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return rec.sess, true
}

// GetWithSessionID is the variant CallAgentRPC uses: returns both
// the live Session and its server-generated id in one lookup so the
// per-call log line and the StreamHeader can carry the same value
// without a second lock acquisition.
func (s *AgentLinkService) GetWithSessionID(agentID string) (sess *link.Session, sessionID string, ok bool) {
	s.mu.RLock()
	rec, ok := s.links[agentID]
	s.mu.RUnlock()
	if !ok {
		return nil, "", false
	}
	return rec.sess, rec.sessionID, true
}

// SessionIDFor returns just the session_id for agentID. Empty when
// the agent isn't registered. Cheap for handlers that only need the
// id for a log line.
func (s *AgentLinkService) SessionIDFor(agentID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if rec, ok := s.links[agentID]; ok {
		return rec.sessionID
	}
	return ""
}

// All returns a defensive copy of the registry. Callers iterate
// the returned map freely; mutating it doesn't affect the
// registry.
func (s *AgentLinkService) All() map[string]*link.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]*link.Session, len(s.links))
	for k, v := range s.links {
		out[k] = v.sess
	}
	return out
}

// CloseAll closes every registered session and empties the registry.
// Server shutdown calls this before http.Server.Shutdown so the
// hijacked-WS accept loops in handler_agent_link_v2 unblock —
// otherwise the 30s grace window expires waiting on yamux Accepts
// that nothing else would ever unblock. Idempotent.
func (s *AgentLinkService) CloseAll() {
	s.mu.Lock()
	links := s.links
	s.links = make(map[string]*linkRecord)
	s.mu.Unlock()
	for _, rec := range links {
		_ = rec.sess.Close()
	}
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

// newLinkSessionID returns a 16-hex-char (8 byte) random id used to
// identify a single agent link instance in logs. Long enough that
// short-lived reconnects don't collide; short enough to fit on one
// log line. Not a wire identifier — never authenticates anything.
func newLinkSessionID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(b[:])
}
