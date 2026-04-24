package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/app"
	"github.com/WangYihang/Platypus/internal/core"
)

// setupWSRouter wires /notify, /ws/:hash and /api/v1/ws/ticket on a fresh
// app context. Returns the engine, an Auth, and a valid Bearer token.
func setupWSRouter(t *testing.T) (*gin.Engine, *Auth, string) {
	t.Helper()
	core.Ctx = app.New(nil)
	auth := NewAuth()
	r := gin.New()
	RegisterV1Routes(r, auth)
	RegisterWebSocketRoutes(r, auth)
	return r, auth, auth.CreateToken()
}

func TestWSTicket_IssueAndConsume(t *testing.T) {
	s := newWSTicketStore()
	tk := s.Issue()
	if len(tk) < 32 {
		t.Fatalf("issued ticket too short: %q", tk)
	}
	if !s.Consume(tk) {
		t.Fatalf("Consume(%q) → false on first use; want true", tk)
	}
	if s.Consume(tk) {
		t.Fatalf("Consume(%q) → true on second use; want false (one-shot)", tk)
	}
}

func TestWSTicket_Expiry(t *testing.T) {
	s := newWSTicketStore()
	tk := s.Issue()
	s.expireTicket(tk)
	if s.Consume(tk) {
		t.Fatalf("Consume(%q) succeeded on expired ticket; want false", tk)
	}
}

func TestWSTicket_InvalidRejected(t *testing.T) {
	s := newWSTicketStore()
	if s.Consume("bogus") {
		t.Fatalf("Consume(bogus) → true; want false")
	}
	if s.Consume("") {
		t.Fatalf("Consume(\"\") → true; want false")
	}
}

// TestWSTicketEndpoint_RequiresBearer ensures the ticket-issuing endpoint
// sits behind Bearer auth. Otherwise anyone could mint a ticket and skip WS
// auth entirely.
func TestWSTicketEndpoint_RequiresBearer(t *testing.T) {
	r, _, _ := setupWSRouter(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/ws/ticket", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("POST /api/v1/ws/ticket without Bearer → %d; want 401", w.Code)
	}
}

func TestWSTicketEndpoint_ReturnsTicketWithBearer(t *testing.T) {
	r, _, tok := setupWSRouter(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/ws/ticket", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/ws/ticket with Bearer → %d; want 200: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"ticket"`) {
		t.Fatalf("response missing ticket field: %s", w.Body.String())
	}
}

// TestWebSocketAuth_RejectsUnauthenticated asserts that /notify
// bounces unauthenticated requests at the HTTP layer — before any
// WS upgrade. The legacy /ws/:hash terminal was removed; v2
// terminal sits at /api/v1/terminal/:agent_id/ws and carries its
// own mTLS-based auth inside handler_terminal_v2.
func TestWebSocketAuth_RejectsUnauthenticated(t *testing.T) {
	r, _, _ := setupWSRouter(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/notify", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("GET /notify without auth → %d; want 401", w.Code)
	}
}

func TestWebSocketAuth_AcceptsBearer(t *testing.T) {
	r, _, tok := setupWSRouter(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/notify", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	// No upgrade headers → melody returns !=200; the assertion is
	// narrower: we didn't bounce at the auth layer.
	if w.Code == http.StatusUnauthorized {
		t.Errorf("GET /notify with Bearer → 401; middleware should have accepted: %s", w.Body.String())
	}
}

func TestWebSocketAuth_AcceptsValidTicketOnce(t *testing.T) {
	r, auth, _ := setupWSRouter(t)
	tk := auth.IssueWSTicket()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/notify?ticket="+tk, nil)
	r.ServeHTTP(w, req)
	if w.Code == http.StatusUnauthorized {
		t.Fatalf("first use of valid ticket → 401; want acceptance: %s", w.Body.String())
	}

	// Reuse — middleware rejects.
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/notify?ticket="+tk, nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("ticket reuse → %d; want 401", w2.Code)
	}
}

func TestWebSocketAuth_RejectsBogusTicket(t *testing.T) {
	r, _, _ := setupWSRouter(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/notify?ticket=deadbeef", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bogus ticket → %d; want 401", w.Code)
	}
}
