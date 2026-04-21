package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
