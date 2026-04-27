package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/user"
)

// RequireAuthWS is the WebSocket-friendly auth middleware. Browsers
// can't set Authorization on a WS upgrade, so it accepts:
//
//  1. Authorization: Bearer <token>            — native clients
//  2. Sec-WebSocket-Protocol: ..., Bearer.<tok> — browser-friendly
//
// A previously-supported third path (?access_token=<tok>) is rejected
// since security audit M3: query strings end up in nginx / cloudflare
// access logs and HTTP referer headers, and a 30-day session token
// in either of those is a long-lived credential leak.
//
// The tests below cover all three carriers explicitly so a future
// maintainer can't reintroduce the query-string fallback without
// noticing.
func mountProtectedWS(rb *RBAC, mw ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	chain := append(mw, func(c *gin.Context) {
		claims, ok := ClaimsFromContext(c)
		if !ok {
			c.String(http.StatusInternalServerError, "no claims")
			return
		}
		c.String(http.StatusOK, string(claims.Role))
	})
	r.GET("/ws", chain...)
	_ = rb // silence unused parameter linter when chain is built externally
	return r
}

func TestRequireAuthWS_AcceptsAuthorizationHeader(t *testing.T) {
	rb, db := rbacTestSetup(t)
	r := mountProtectedWS(rb, rb.RequireAuthWS())
	tok := mintBearerForUserID(t, db, "u1", user.RoleOperator)

	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestRequireAuthWS_AcceptsSecWebSocketProtocol(t *testing.T) {
	rb, db := rbacTestSetup(t)
	r := mountProtectedWS(rb, rb.RequireAuthWS())
	tok := mintBearerForUserID(t, db, "u1", user.RoleViewer)

	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Sec-WebSocket-Protocol", "tty, Bearer."+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

// M3: ?access_token=<tok> must NOT authenticate, even with a
// completely valid token. Query-string credentials leak via referer /
// access logs / proxy logs and have to be eliminated as an option,
// not just deprioritised.
func TestRequireAuthWS_RejectsAccessTokenQueryString(t *testing.T) {
	rb, db := rbacTestSetup(t)
	r := mountProtectedWS(rb, rb.RequireAuthWS())
	tok := mintBearerForUserID(t, db, "u1", user.RoleAdmin)

	target := "/ws?access_token=" + url.QueryEscape(tok)
	req := httptest.NewRequest("GET", target, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("?access_token= must be rejected; got status=%d body=%s",
			w.Code, w.Body.String())
	}
}

func TestRequireAuthWS_NoCredentialIs401(t *testing.T) {
	rb, _ := rbacTestSetup(t)
	r := mountProtectedWS(rb, rb.RequireAuthWS())

	req := httptest.NewRequest("GET", "/ws", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
