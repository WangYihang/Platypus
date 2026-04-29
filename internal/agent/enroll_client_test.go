package agent

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// Enroll wraps POST /api/v1/agents/enroll: build a CSR, marshal a
// v2pb.EnrollRequest, send it, parse the v2pb.EnrollResponse, and
// return the freshly-minted Identity (cert + private key + CA).
//
// Tests drive it against a stub *httptest.Server so we can assert
// request shape without depending on the real enrollment service.

// fakeEnrollServer returns a *httptest.Server that behaves like the
// real /api/v1/agents/enroll endpoint: on receipt of a valid
// EnrollRequest it emits an EnrollResponse with caller-supplied
// agent_id / project_id and a pass-through of the CSR pubkey in a
// dummy "cert" blob so the test can verify the roundtrip.
func fakeEnrollServer(t *testing.T, wantPAT string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/agents/enroll", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method %s", r.Method)
			http.Error(w, "wrong method", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		var req v2pb.EnrollRequest
		if err := proto.Unmarshal(body, &req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Pat != wantPAT {
			http.Error(w, "bad pat", http.StatusUnauthorized)
			return
		}
		if !bytes.Contains(req.CsrPem, []byte("CERTIFICATE REQUEST")) {
			t.Errorf("CsrPem missing PEM header; got %q", req.CsrPem)
		}
		resp := &v2pb.EnrollResponse{
			AgentId:         "agent-stub",
			ProjectId:       "proj-stub",
			CertPem:         []byte("-----BEGIN CERTIFICATE-----\nSTUB\n-----END CERTIFICATE-----\n"),
			CaPem:           []byte("-----BEGIN CERTIFICATE-----\nSTUBCA\n-----END CERTIFICATE-----\n"),
			CertExpiresUnix: 1_800_000_000,
		}
		out, err := proto.Marshal(resp)
		if err != nil {
			http.Error(w, "marshal", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/x-protobuf-platypus-v2")
		_, _ = w.Write(out)
	})
	return httptest.NewServer(mux)
}

func TestEnroll_HappyPath(t *testing.T) {
	srv := fakeEnrollServer(t, "plt_goodpat")
	defer srv.Close()

	id, err := Enroll(context.Background(), EnrollOptions{
		ServerURL:       srv.URL,
		PAT:             "plt_goodpat",
		Hostname:        "unit-test",
		BuildVersion:    "1.5.1-test",
		BuildCommit:     "deadbee",
		BuildDate:       "2025-01-01T00:00:00Z",
		ProtocolVersion: 1,
	})
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	if id.AgentID != "agent-stub" {
		t.Fatalf("AgentID = %q; want agent-stub", id.AgentID)
	}
	if id.ProjectID != "proj-stub" {
		t.Fatalf("ProjectID = %q; want proj-stub", id.ProjectID)
	}
	if len(id.PrivateKey) != ed25519.PrivateKeySize {
		t.Fatalf("PrivateKey len = %d; want %d", len(id.PrivateKey), ed25519.PrivateKeySize)
	}
	if !strings.Contains(string(id.Identity.CertPEM), "STUB") {
		t.Fatalf("CertPEM = %q; want stub from server", id.Identity.CertPEM)
	}
	if id.ExpiresAt.Unix() != 1_800_000_000 {
		t.Fatalf("ExpiresAt.Unix = %d; want 1_800_000_000", id.ExpiresAt.Unix())
	}
}

func TestEnroll_UnauthorizedPAT(t *testing.T) {
	srv := fakeEnrollServer(t, "plt_realpat")
	defer srv.Close()

	_, err := Enroll(context.Background(), EnrollOptions{
		ServerURL: srv.URL,
		PAT:       "plt_wrongpat",
	})
	if err == nil {
		t.Fatal("Enroll err = nil; want non-nil for 401")
	}
	if !strings.Contains(err.Error(), "401") && !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("err %q does not mention 401 / unauthorized", err.Error())
	}
}

func TestEnroll_ServerDown(t *testing.T) {
	// Point at an address that should immediately refuse. 127.0.0.1:1
	// has reserved-for-"user space" status in RFC 6335 and is unused
	// on most systems.
	_, err := Enroll(context.Background(), EnrollOptions{
		ServerURL: "http://127.0.0.1:1",
		PAT:       "plt_whatever",
	})
	if err == nil {
		t.Fatal("Enroll err = nil; want non-nil when server unreachable")
	}
}

func TestEnroll_RequiresPATAndURL(t *testing.T) {
	if _, err := Enroll(context.Background(), EnrollOptions{PAT: "plt_x"}); err == nil {
		t.Fatal("Enroll with no ServerURL should fail")
	}
	if _, err := Enroll(context.Background(), EnrollOptions{ServerURL: "http://x"}); err == nil {
		t.Fatal("Enroll with no PAT should fail")
	}
}
