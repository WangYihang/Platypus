package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/app"
	"github.com/WangYihang/Platypus/internal/core"
)

func setupListenersV1Router(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	core.Ctx = app.New(nil)
	auth := NewAuth()
	r := gin.New()
	RegisterV1Routes(r, auth)
	return r, auth.CreateToken()
}

func TestListenersV1_ListEmpty(t *testing.T) {
	r, tok := setupListenersV1Router(t)
	w := smokeReq(t, r, "GET", "/api/v1/listeners", tok, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"listeners":`) {
		t.Fatalf("response missing listeners field: %s", w.Body.String())
	}
}

func TestListenersV1_GetNotFound(t *testing.T) {
	r, tok := setupListenersV1Router(t)
	w := smokeReq(t, r, "GET", "/api/v1/listeners/bogus", tok, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestListenersV1_CreateBadBody(t *testing.T) {
	r, tok := setupListenersV1Router(t)
	w := smokeReq(t, r, "POST", "/api/v1/listeners", tok, "{}")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty body, got %d", w.Code)
	}
}

func TestListenersV1_CreateBadPort(t *testing.T) {
	r, tok := setupListenersV1Router(t)
	w := smokeReq(t, r, "POST", "/api/v1/listeners", tok, `{"host":"0.0.0.0","port":99999}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for out-of-range port, got %d", w.Code)
	}
}

func TestListenersV1_DeleteNotFound(t *testing.T) {
	r, tok := setupListenersV1Router(t)
	w := smokeReq(t, r, "DELETE", "/api/v1/listeners/bogus", tok, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestListenersV1_SessionsNotFound(t *testing.T) {
	r, tok := setupListenersV1Router(t)
	w := smokeReq(t, r, "GET", "/api/v1/listeners/bogus/sessions", tok, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestListenersV1_AuthRequired(t *testing.T) {
	r, _ := setupListenersV1Router(t)
	cases := []struct{ method, path, body string }{
		{"GET", "/api/v1/listeners", ""},
		{"GET", "/api/v1/listeners/x", ""},
		{"POST", "/api/v1/listeners", `{"host":"0.0.0.0","port":1337}`},
		{"DELETE", "/api/v1/listeners/x", ""},
		{"GET", "/api/v1/listeners/x/sessions", ""},
	}
	for _, tc := range cases {
		w := smokeReq(t, r, tc.method, tc.path, "", tc.body)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token → %d; want 401", tc.method, tc.path, w.Code)
		}
	}
}
