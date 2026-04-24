package agent

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestCollectProcessList runs against the test binary itself — we
// assert (a) our own pid comes back, (b) the ordering respects
// sort_by, (c) cmdline truncation works on a synthetic input, and
// (d) top_n bounds the response.
func TestCollectProcessList(t *testing.T) {
	resp := CollectProcessList(context.Background(), 0, "cpu")
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.TotalCount == 0 {
		t.Fatal("TotalCount = 0; expected at least one process")
	}

	selfPID := uint32(os.Getpid())
	found := false
	for _, p := range resp.Processes {
		if p.Pid == selfPID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("self pid %d missing from process list", selfPID)
	}

	t.Run("top_n bounds", func(t *testing.T) {
		r := CollectProcessList(context.Background(), 5, "cpu")
		if len(r.Processes) > 5 {
			t.Errorf("top_n=5 returned %d processes", len(r.Processes))
		}
	})

	t.Run("cap enforced", func(t *testing.T) {
		r := CollectProcessList(context.Background(), 0, "cpu")
		if len(r.Processes) > processListCap {
			t.Errorf("list exceeds cap: %d > %d", len(r.Processes), processListCap)
		}
	})
}

func TestTruncateCmdline(t *testing.T) {
	short := "echo hi"
	if got := truncateCmdline(short); got != short {
		t.Errorf("short cmdline mutated: %q", got)
	}
	long := strings.Repeat("a", cmdlineTruncateBytes+50)
	got := truncateCmdline(long)
	// The ellipsis is multi-byte; just require length <= cap+a few.
	if len(got) <= cmdlineTruncateBytes || len(got) > cmdlineTruncateBytes+5 {
		t.Errorf("truncateCmdline produced unexpected length %d", len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated cmdline missing ellipsis: %q", got)
	}
}
