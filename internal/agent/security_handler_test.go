package agent

import (
	"context"
	"testing"
	"time"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// HandleSecurityScan must always populate StartedAtUnix even when no
// checkers are registered (e.g. on non-Linux builds), so callers can
// rely on the timestamp field for ordering / freshness checks.
func TestHandleSecurityScan_AlwaysReturnsStartedAt(t *testing.T) {
	before := time.Now().Add(-time.Second).Unix()
	resp := HandleSecurityScan(context.Background(), &v2pb.SecurityScanRequest{})
	after := time.Now().Add(time.Second).Unix()
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.GetStartedAtUnix() < before || resp.GetStartedAtUnix() > after {
		t.Fatalf("StartedAtUnix=%d out of [%d,%d]", resp.GetStartedAtUnix(), before, after)
	}
}

func TestHandleSecurityScan_NilRequestSafe(t *testing.T) {
	resp := HandleSecurityScan(context.Background(), nil)
	if resp == nil {
		t.Fatal("nil request should still produce a response")
	}
}

func TestDeriveCheckID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ssh.permitrootlogin", "ssh"},
		{"kernel.version.eol", "kernel.version"},
		{"plain", "plain"},
		{"", ""},
	}
	for _, c := range cases {
		if got := deriveCheckID(c.in); got != c.want {
			t.Errorf("deriveCheckID(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}
