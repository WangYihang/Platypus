package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// probeReqWithPath fires a JSON request against handler r with an optional
// Bearer token. It exists because every v1 handler test wants to spin
// requests with a token + JSON body, and each adding its own variant got
// ugly fast. Keep additions to this helper minimal — one responsibility.
func probeReqWithPath(r http.Handler, method, path, bearer string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// testCtx returns a context suitable for test DB calls where the request
// context isn't handy (e.g. post-hoc assertions on what the handler wrote).
func testCtx() context.Context {
	return context.Background()
}

// mintSessionForTest seeds an auth_tokens row with kind='user_session'
// and returns its plaintext bearer. Replaces the old "issuer.IssueAccess"
// pattern: tests no longer mint JWTs, they mint sessions, exactly the
// way Login does in production.
func mintSessionForTest(t *testing.T, db *storage.DB, u *user.User) string {
	t.Helper()
	id, _, hash, plaintext, err := optoken.Generate(optoken.UserSessionPrefix)
	if err != nil {
		t.Fatalf("optoken.Generate: %v", err)
	}
	now := time.Now().UTC()
	s := &storage.UserSession{
		TokenID:       id,
		SecretHash:    hash,
		UserID:        u.ID,
		CreatedAt:     now,
		ExpiresAt:     now.Add(SessionHardTTL),
		IdleExpiresAt: now.Add(SessionIdleWindow),
	}
	if err := db.AuthTokens().CreateSession(context.Background(), s); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return plaintext
}

// mintSessionForUserID is the by-id variant for tests where seeding a
// real users row would be excessive. The auth_tokens FK still
// requires the row to exist, so callers must have created the user
// (typically via seedUserForAPITest).
func mintSessionForUserID(t *testing.T, db *storage.DB, userID string) string {
	t.Helper()
	return mintSessionForTest(t, db, &user.User{ID: userID})
}
