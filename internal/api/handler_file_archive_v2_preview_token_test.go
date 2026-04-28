package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// signerForTests constructs a PreviewSigner with a deterministic
// stand-in clock so tests can drive expiry / boundary behaviour without
// time.Sleep. Returned signer's TTL is the production default.
func signerForTests(t *testing.T) *PreviewSigner {
	t.Helper()
	s, err := NewPreviewSigner()
	if err != nil {
		t.Fatalf("NewPreviewSigner: %v", err)
	}
	return s
}

// registerArchiveRoutesWithSigner mirrors registerArchiveRoutes but
// wires a PreviewSigner into FileArchiveDeps so /fs/read accepts the
// preview-token URL form. Used by the preview-token suite below.
func (a *archiveTestAgent) registerArchiveRoutesWithSigner(r *gin.Engine, signer *PreviewSigner) {
	deps := FileArchiveDeps{
		Service:       a.svc,
		RBAC:          a.fixture.RBAC,
		Recorder:      a.recorder,
		Broadcaster:   nil,
		Cancels:       a.cancels,
		IDGenerator:   func() string { return "ft-test-1" },
		PreviewSigner: signer,
	}
	if a.hosts != nil {
		deps.Hosts = a.hosts
	}
	RegisterV2FileArchiveRoutes(r, deps)
}

// fsReadURLWithToken builds the canonical browser-direct URL the
// frontend's <video src=...> would use.
func fsReadURLWithToken(srvURL, prefix, path, tok string, exp int64) string {
	q := url.Values{}
	q.Set("path", path)
	q.Set("exp", strconv.FormatInt(exp, 10))
	q.Set("preview_token", tok)
	return srvURL + prefix + "/fs/read?" + q.Encode()
}

// TestFileRead_PreviewToken_Success exercises the happy path: a
// freshly-minted token authorises a GET that carries no Authorization
// header, and the response body matches the requested file.
func TestFileRead_PreviewToken_Success(t *testing.T) {
	want := bytes.Repeat([]byte("AB"), 32)
	a := setupArchiveAgentWithRead(t, "agent-prev-ok", want)
	signer := signerForTests(t)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	a.registerArchiveRoutesWithSigner(r, signer)
	srv := httptest.NewServer(r)
	defer srv.Close()

	const path = "/tmp/movie.mp4"
	tok, exp := signer.Sign(a.fixture.ProjectID, a.fixture.AgentID, path)

	req, _ := http.NewRequest(http.MethodGet,
		fsReadURLWithToken(srv.URL, a.fixture.URL(""), path, tok, exp), nil)
	// Deliberately no Authorization header — the whole point of the
	// preview-token path is to support browser-native <video> elements
	// that can't set one.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 200; body=%s", resp.StatusCode, b)
	}
	got, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(got, want) {
		t.Fatalf("body mismatch: got %d bytes want %d", len(got), len(want))
	}
}

// TestFileRead_PreviewToken_Tampered flips a byte of the signature and
// asserts the request is rejected. Without this guard a captured URL
// could be edited to read sibling files.
func TestFileRead_PreviewToken_Tampered(t *testing.T) {
	a := setupArchiveAgentWithRead(t, "agent-prev-tamper", []byte("payload"))
	signer := signerForTests(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	a.registerArchiveRoutesWithSigner(r, signer)
	srv := httptest.NewServer(r)
	defer srv.Close()

	tok, exp := signer.Sign(a.fixture.ProjectID, a.fixture.AgentID, "/x")
	bad := tok[:len(tok)-1]
	if strings.HasSuffix(tok, "A") {
		bad += "B"
	} else {
		bad += "A"
	}

	req, _ := http.NewRequest(http.MethodGet,
		fsReadURLWithToken(srv.URL, a.fixture.URL(""), "/x", bad, exp), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 401; body=%s", resp.StatusCode, b)
	}
}

// TestFileRead_PreviewToken_Expired ages the signer's clock past the
// embedded exp before issuing the request.
func TestFileRead_PreviewToken_Expired(t *testing.T) {
	a := setupArchiveAgentWithRead(t, "agent-prev-exp", []byte("payload"))
	signer := signerForTests(t)
	// Issue with the natural clock so exp is sane, then advance the
	// signer's clock so verification fails.
	tok, exp := signer.Sign(a.fixture.ProjectID, a.fixture.AgentID, "/x")
	signer.now = func() time.Time { return time.Unix(exp+1, 0) }

	gin.SetMode(gin.TestMode)
	r := gin.New()
	a.registerArchiveRoutesWithSigner(r, signer)
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet,
		fsReadURLWithToken(srv.URL, a.fixture.URL(""), "/x", tok, exp), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 401; body=%s", resp.StatusCode, b)
	}
}

// TestFileRead_PreviewToken_PathMismatch confirms a token minted for
// /a cannot read /b on the same agent — i.e. the signature actually
// binds to path, not just exp+secret.
func TestFileRead_PreviewToken_PathMismatch(t *testing.T) {
	a := setupArchiveAgentWithRead(t, "agent-prev-path", []byte("payload"))
	signer := signerForTests(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	a.registerArchiveRoutesWithSigner(r, signer)
	srv := httptest.NewServer(r)
	defer srv.Close()

	tok, exp := signer.Sign(a.fixture.ProjectID, a.fixture.AgentID, "/a")
	req, _ := http.NewRequest(http.MethodGet,
		fsReadURLWithToken(srv.URL, a.fixture.URL(""), "/b", tok, exp), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 401; body=%s", resp.StatusCode, b)
	}
}

// TestFileRead_NoAuth_NoToken keeps the regression honest: neither
// Bearer nor preview_token still gets you 401, not a silently-allowed
// pass.
func TestFileRead_NoAuth_NoToken(t *testing.T) {
	a := setupArchiveAgentWithRead(t, "agent-prev-noauth", []byte("x"))
	gin.SetMode(gin.TestMode)
	r := gin.New()
	a.registerArchiveRoutesWithSigner(r, signerForTests(t))
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet,
		srv.URL+a.fixture.URL("/fs/read?path=/x"), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 401; body=%s", resp.StatusCode, b)
	}
}

// TestFileRead_PreviewTokenMint covers the POST /fs/preview-token
// endpoint that frontends use to mint a URL before passing it to a
// <video src=...>. The endpoint is bearer-auth gated (so an
// unauthenticated request can't mint anything) and returns a token
// that immediately verifies under the same signer.
func TestFileRead_PreviewTokenMint(t *testing.T) {
	a := setupArchiveAgentWithRead(t, "agent-prev-mint", []byte("x"))
	signer := signerForTests(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	a.registerArchiveRoutesWithSigner(r, signer)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"path": "/var/log/big.bin"})
	req, _ := http.NewRequest(http.MethodPost,
		srv.URL+a.fixture.URL("/fs/preview-token"), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+a.fixture.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 200; body=%s", resp.StatusCode, b)
	}
	var minted struct {
		Token string `json:"token"`
		Exp   int64  `json:"exp"`
		URL   string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&minted); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !signer.Verify(a.fixture.ProjectID, a.fixture.AgentID, "/var/log/big.bin", minted.Exp, minted.Token) {
		t.Fatalf("freshly-minted token did not verify")
	}
	// The returned URL should be ready to drop into a <video src> —
	// path/exp/preview_token all encoded in the query.
	if !strings.Contains(minted.URL, "preview_token=") || !strings.Contains(minted.URL, "/fs/read") {
		t.Fatalf("URL missing expected fragments: %q", minted.URL)
	}
}

// TestFileRead_PreviewTokenMint_RequiresAuth proves the mint endpoint
// itself isn't a backdoor: an unauthenticated request can't get a
// token at all.
func TestFileRead_PreviewTokenMint_RequiresAuth(t *testing.T) {
	a := setupArchiveAgentWithRead(t, "agent-prev-mint-anon", []byte("x"))
	gin.SetMode(gin.TestMode)
	r := gin.New()
	a.registerArchiveRoutesWithSigner(r, signerForTests(t))
	srv := httptest.NewServer(r)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"path": "/var/log/big.bin"})
	req, _ := http.NewRequest(http.MethodPost,
		srv.URL+a.fixture.URL("/fs/preview-token"), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// Deliberately no Authorization header.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 401; body=%s", resp.StatusCode, b)
	}
}
