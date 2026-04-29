package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/storage"
)

// TestAgentLinkHandler_PendingApprovalReturns425 walks the approval
// gate happy path: a host with status=pending makes the link handler
// short-circuit before WS upgrade with 425 Too Early and a header the
// agent client can read to drive its retry loop. The cert chain is
// valid — only the host-level approval flag stops the link.
func TestAgentLinkHandler_PendingApprovalReturns425(t *testing.T) {
	pki := mintAgentLinkPKI(t, "agent-pending", "proj-1")

	db, srv, cleanup := startApprovalLinkServer(t, pki)
	defer cleanup()

	// Insert a hosts row in pending state for this agent.
	ctx := context.Background()
	if _, err := db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: "proj-1", AgentID: "agent-pending",
		MachineID: "m-pending", Fingerprint: "fp-pending",
		Hostname:        "pending-host",
		SeenAt:          time.Now().UTC(),
		InitialApproval: storage.HostApprovalPending,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	clientTLS := &tls.Config{
		Certificates: []tls.Certificate{pki.clientTLS},
		RootCAs:      pki.caPool,
		MinVersion:   tls.VersionTLS12,
	}
	wsURL := strings.Replace(srv.URL, "https://", "wss://", 1) + "/api/v1/agent/link"
	_, err := link.Dial(dialCtx, link.DialOptions{URL: wsURL, TLSConfig: clientTLS})
	if !errors.Is(err, link.ErrPendingApproval) {
		t.Fatalf("err = %v, want ErrPendingApproval", err)
	}
}

// TestAgentLinkHandler_RejectedReturns403 verifies the rejected-state
// path: server returns 403 with the rejected header, agent classifies
// as ErrApprovalRejected (terminal — no retry).
func TestAgentLinkHandler_RejectedReturns403(t *testing.T) {
	pki := mintAgentLinkPKI(t, "agent-rejected", "proj-1")

	db, srv, cleanup := startApprovalLinkServer(t, pki)
	defer cleanup()

	ctx := context.Background()
	if _, err := db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: "proj-1", AgentID: "agent-rejected",
		MachineID: "m-rej", Fingerprint: "fp-rej",
		Hostname:        "rejected-host",
		SeenAt:          time.Now().UTC(),
		InitialApproval: storage.HostApprovalRejected,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	clientTLS := &tls.Config{
		Certificates: []tls.Certificate{pki.clientTLS},
		RootCAs:      pki.caPool,
		MinVersion:   tls.VersionTLS12,
	}
	wsURL := strings.Replace(srv.URL, "https://", "wss://", 1) + "/api/v1/agent/link"
	_, err := link.Dial(dialCtx, link.DialOptions{URL: wsURL, TLSConfig: clientTLS})
	if !errors.Is(err, link.ErrApprovalRejected) {
		t.Fatalf("err = %v, want ErrApprovalRejected", err)
	}
}

// TestAgentLinkHandler_ApprovedAcceptsLink confirms the approval
// gate is opt-in to the negative — once the host is approved, the
// link upgrade succeeds end-to-end and the session lands in the
// AgentLinkService.
func TestAgentLinkHandler_ApprovedAcceptsLink(t *testing.T) {
	pki := mintAgentLinkPKI(t, "agent-approved", "proj-1")

	db, srv, cleanup := startApprovalLinkServer(t, pki)
	defer cleanup()

	ctx := context.Background()
	if _, err := db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: "proj-1", AgentID: "agent-approved",
		MachineID: "m-ok", Fingerprint: "fp-ok",
		Hostname:        "ok-host",
		SeenAt:          time.Now().UTC(),
		InitialApproval: storage.HostApprovalApproved,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	clientTLS := &tls.Config{
		Certificates: []tls.Certificate{pki.clientTLS},
		RootCAs:      pki.caPool,
		MinVersion:   tls.VersionTLS12,
	}
	wsURL := strings.Replace(srv.URL, "https://", "wss://", 1) + "/api/v1/agent/link"
	sess, err := link.Dial(dialCtx, link.DialOptions{URL: wsURL, TLSConfig: clientTLS})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer sess.Close()
}

// startApprovalLinkServer wires an in-memory DB into a fresh
// AgentLinkHandler + httptest TLS listener. Returns the DB so tests
// can seed approval rows, the server URL, and a cleanup func.
func startApprovalLinkServer(t *testing.T, pki *agentLinkTestPKI) (*storage.DB, *httptest.Server, func()) {
	t.Helper()

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}

	// Seed a project row so the FK on hosts.project_id holds. The
	// project_id used here matches what mintAgentLinkPKI baked into
	// the cert URI SAN.
	ctx := context.Background()
	systemUserID, err := storage.EnsureSystemUser(ctx, db)
	if err != nil {
		t.Fatalf("EnsureSystemUser: %v", err)
	}
	if err := db.Projects().Create(ctx, &storage.Project{
		ID: "proj-1", Slug: "proj-1", Name: "Approval test",
		CreatedAt: time.Now().UTC(), CreatedBy: systemUserID,
	}); err != nil {
		t.Fatalf("Projects.Create: %v", err)
	}

	svc := core.NewAgentLinkService()
	h := NewAgentLinkHandler(svc, func() *x509.CertPool { return pki.caPool }).WithDB(db)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterV2AgentLinkRoute(r, h)

	srv := httptest.NewUnstartedServer(r)
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{pki.serverTLS},
		ClientAuth:   tls.RequestClientCert,
		MinVersion:   tls.VersionTLS12,
	}
	srv.StartTLS()

	cleanup := func() {
		srv.Close()
		_ = db.Close()
	}
	return db, srv, cleanup
}

// TestLinkDial_PendingApprovalClassification: the link.Dial classifier
// triggers off both X-Platypus-Approval-Status header (preferred) and
// the bare HTTP status code (fallback for older servers).
func TestLinkDial_PendingApprovalClassification(t *testing.T) {
	t.Run("header is authoritative", func(t *testing.T) {
		// 425 Too Early is wired in the production handler; the header
		// is independent. Even if a future server returned 425 with no
		// header (or 200 with a header), the classifier should defer
		// to the header value first.
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Platypus-Approval-Status", "rejected")
			w.WriteHeader(http.StatusForbidden)
		}))
		defer srv.Close()
		wsURL := strings.Replace(srv.URL, "https://", "wss://", 1)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		// Use the server's self-signed root so TLS handshake passes;
		// pull it from srv.TLS.Certificates[0] via Certificate().
		pool := x509.NewCertPool()
		if c := srv.Certificate(); c != nil {
			pool.AddCert(c)
		}
		_, err := link.Dial(ctx, link.DialOptions{
			URL:       wsURL,
			TLSConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		})
		if !errors.Is(err, link.ErrApprovalRejected) {
			t.Fatalf("err = %v, want ErrApprovalRejected", err)
		}
	})

	t.Run("status code fallback", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooEarly)
		}))
		defer srv.Close()
		wsURL := strings.Replace(srv.URL, "https://", "wss://", 1)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		pool := x509.NewCertPool()
		if c := srv.Certificate(); c != nil {
			pool.AddCert(c)
		}
		_, err := link.Dial(ctx, link.DialOptions{
			URL:       wsURL,
			TLSConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		})
		if !errors.Is(err, link.ErrPendingApproval) {
			t.Fatalf("err = %v, want ErrPendingApproval", err)
		}
	})
}

// silence unused import for deps that may shift.
var _ = url.Parse
