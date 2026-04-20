package api

import (
	"sync"
	"time"
)

// wsTicketTTL is how long a freshly-issued ticket remains valid. Short enough
// that a leaked ticket is useless minutes later; long enough that normal
// "click Terminal → WS opens" flows never time out.
const wsTicketTTL = 60 * time.Second

// wsTicketStore is a one-shot, short-lived ticket issuer for WebSocket auth.
// Browsers cannot set arbitrary headers on WebSocket upgrade requests, so we
// offer a ticket-in-query-string alternative to Bearer headers. Tickets are:
//
//   - random 32-byte hex strings (64 chars)
//   - valid for wsTicketTTL
//   - one-shot (Consume deletes on success)
//   - stored in memory (single-node C2 assumption)
//
// The zero value is NOT usable — use newWSTicketStore.
type wsTicketStore struct {
	mu      sync.Mutex
	tickets map[string]time.Time
}

func newWSTicketStore() *wsTicketStore {
	return &wsTicketStore{tickets: map[string]time.Time{}}
}

// Issue mints a new ticket and records its expiry. Expired entries are
// opportunistically swept on each Issue call, which is cheap enough for the
// single-operator C2 workload (ticket store rarely exceeds single digits).
func (s *wsTicketStore) Issue() string {
	t := generateRandomHex(32)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tickets[t] = time.Now().Add(wsTicketTTL)
	now := time.Now()
	for k, exp := range s.tickets {
		if now.After(exp) {
			delete(s.tickets, k)
		}
	}
	return t
}

// Consume validates a ticket, deletes it, and reports whether it was valid
// and unexpired. Returns false for unknown, empty, expired, or already-used
// tickets — i.e. the caller can treat false as "reject with 401" without
// discriminating further.
func (s *wsTicketStore) Consume(t string) bool {
	if t == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.tickets[t]
	if !ok {
		return false
	}
	delete(s.tickets, t)
	return time.Now().Before(exp)
}

// expireTicket is a test hook that rewinds a ticket's expiry to the past.
// Production code never calls this.
func (s *wsTicketStore) expireTicket(t string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tickets[t]; ok {
		s.tickets[t] = time.Now().Add(-time.Hour)
	}
}
