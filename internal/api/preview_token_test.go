package api

import (
	"strings"
	"testing"
	"time"
)

// TestPreviewSigner_RoundTrip exercises the happy path: a token signed
// for a (pid, agentID, path) tuple verifies under the same secret with
// the same parameters.
func TestPreviewSigner_RoundTrip(t *testing.T) {
	s, err := NewPreviewSigner()
	if err != nil {
		t.Fatalf("NewPreviewSigner: %v", err)
	}
	const (
		pid     = "proj-1"
		agentID = "agent-1"
		path    = "/tmp/movie.mp4"
	)

	tok, exp := s.Sign(pid, agentID, path)
	if tok == "" || exp == 0 {
		t.Fatalf("Sign returned empty token / exp: %q %d", tok, exp)
	}
	if !s.Verify(pid, agentID, path, exp, tok) {
		t.Fatalf("Verify of freshly-signed token failed")
	}
}

// TestPreviewSigner_TamperedSig flips bits of the signature and ensures
// Verify rejects. Constant-time comparison must rule out byte-level
// timing oracles, but for correctness all we test is "any tamper → no".
func TestPreviewSigner_TamperedSig(t *testing.T) {
	s, _ := NewPreviewSigner()
	tok, exp := s.Sign("p", "a", "/x")
	// Flip the last char so the signature no longer matches.
	bad := tok[:len(tok)-1] + "_"
	if bad == tok {
		// Pathological — tok ended in '_' and our flip was a no-op.
		// Try a different replacement so the test still has signal.
		bad = tok[:len(tok)-1] + "A"
	}
	if s.Verify("p", "a", "/x", exp, bad) {
		t.Fatalf("Verify accepted tampered signature")
	}
}

// TestPreviewSigner_MismatchedFields ensures the signature is bound to
// every input — flipping any of (pid, agentID, path, exp) must reject.
// Without this, a token minted for /etc/passwd could be reused to read
// /etc/shadow on the same agent.
func TestPreviewSigner_MismatchedFields(t *testing.T) {
	s, _ := NewPreviewSigner()
	tok, exp := s.Sign("p", "a", "/x")

	cases := []struct {
		name           string
		pid, aid, path string
		exp            int64
	}{
		{"different pid", "p2", "a", "/x", exp},
		{"different agentID", "p", "a2", "/x", exp},
		{"different path", "p", "a", "/y", exp},
		{"different exp", "p", "a", "/x", exp + 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if s.Verify(tc.pid, tc.aid, tc.path, tc.exp, tok) {
				t.Fatalf("Verify accepted token under %s", tc.name)
			}
		})
	}
}

// TestPreviewSigner_Expired pins the expiry-clock check. We freeze the
// signer's clock to issue a token, then advance past the TTL and assert
// Verify rejects despite the signature still being valid.
func TestPreviewSigner_Expired(t *testing.T) {
	s, _ := NewPreviewSigner()
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	tok, exp := s.Sign("p", "a", "/x")

	// Advance one second past the embedded exp.
	s.now = func() time.Time { return time.Unix(exp+1, 0) }
	if s.Verify("p", "a", "/x", exp, tok) {
		t.Fatalf("Verify accepted expired token")
	}
}

// TestPreviewSigner_TTLBoundary documents that exactly-at-exp is still
// considered live. Off-by-one differences between issuer and verifier
// clocks are tolerated as long as the token's exp >= now.
func TestPreviewSigner_TTLBoundary(t *testing.T) {
	s, _ := NewPreviewSigner()
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	tok, exp := s.Sign("p", "a", "/x")

	// Verifier sees the wall-clock exactly at exp.
	s.now = func() time.Time { return time.Unix(exp, 0) }
	if !s.Verify("p", "a", "/x", exp, tok) {
		t.Fatalf("Verify rejected token at exp boundary")
	}
}

// TestPreviewSigner_DistinctSecrets ensures two independently constructed
// signers cannot verify each other's tokens — the per-process random
// secret is the trust root.
func TestPreviewSigner_DistinctSecrets(t *testing.T) {
	a, _ := NewPreviewSigner()
	b, _ := NewPreviewSigner()
	tok, exp := a.Sign("p", "ag", "/x")
	if b.Verify("p", "ag", "/x", exp, tok) {
		t.Fatalf("a's token verified under b's secret")
	}
}

// TestPreviewSigner_TokenShape pins the on-the-wire format so the
// frontend's ?preview_token=... value is URL-safe (no padding, no '+'
// or '/' that need percent-encoding). The format is opaque to callers
// but a regression that introduced ambiguous chars would silently
// double-encode.
func TestPreviewSigner_TokenShape(t *testing.T) {
	s, _ := NewPreviewSigner()
	tok, _ := s.Sign("p", "a", "/x")
	if strings.ContainsAny(tok, "+/=") {
		t.Fatalf("token %q contains chars that need URL-encoding", tok)
	}
	if tok == "" {
		t.Fatalf("token is empty")
	}
}
