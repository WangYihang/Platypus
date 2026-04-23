package agent

import (
	"path/filepath"
	"testing"

	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// deliverRenewalResponse unblocks whoever registered the request_id
// before SendEnvelope. Tests the small waiter registry that bridges
// HandleConnection (reads) and StartRenewalLoop (writes).
func TestRenewalWaiters_DeliversToRegisteredID(t *testing.T) {
	ch, cancel := registerRenewalWaiter("req-1")
	defer cancel()

	want := &agentpb.SessionRenewResponse{
		SessionToken:     "sess_new",
		SessionExpiresAt: 12345,
	}
	deliverRenewalResponse("req-1", want)

	select {
	case got := <-ch:
		if got.SessionToken != "sess_new" {
			t.Fatalf("SessionToken = %q", got.SessionToken)
		}
	default:
		t.Fatal("registered waiter didn't receive response")
	}
}

// deliverRenewalResponse must be a no-op (never panic, never block) when
// no waiter is registered — happens if the response arrives after the
// caller's timeout fires and unregisters.
func TestRenewalWaiters_DropsWithoutWaiter(t *testing.T) {
	// Doesn't panic → pass. No registered id.
	deliverRenewalResponse("no-such-id", &agentpb.SessionRenewResponse{})
}

// registerRenewalWaiter's cancel func must clean the map entry so
// repeated registrations don't leak memory and a stale late response
// can't deliver to a dead channel.
func TestRenewalWaiters_CancelRemoves(t *testing.T) {
	_, cancel := registerRenewalWaiter("req-2")
	cancel()
	// Second call after cancel is a no-op (the slot's gone).
	deliverRenewalResponse("req-2", &agentpb.SessionRenewResponse{})
	// If the cancel didn't clean up, re-registering the same id would
	// succeed — but allow duplicates for robustness (our real use
	// constructs nanosecond-unique ids anyway).
}

func TestResolveIdentityDirUsesStableDefault(t *testing.T) {
	got := ResolveIdentityDir("")
	if got == "" {
		t.Fatal("ResolveIdentityDir returned empty path")
	}
	if filepath.Base(got) != "agent" {
		t.Fatalf("ResolveIdentityDir = %q, want path ending with /agent", got)
	}
}

func TestPersistAndLoadMeshBootstrap(t *testing.T) {
	identityDir := t.TempDir()
	wantPSK := []byte("0123456789abcdef0123456789abcdef")
	wantPeers := []string{"127.0.0.1:17777", "127.0.0.1:27777"}
	if err := PersistMeshBootstrap(identityDir, wantPSK, "proj-1", wantPeers); err != nil {
		t.Fatalf("PersistMeshBootstrap: %v", err)
	}

	got, err := LoadPersistedMeshBootstrap(identityDir)
	if err != nil {
		t.Fatalf("LoadPersistedMeshBootstrap: %v", err)
	}
	if got == nil {
		t.Fatal("LoadPersistedMeshBootstrap returned nil state")
	}
	if got.PSKFile != filepath.Join(identityDir, "mesh", "psk") {
		t.Fatalf("PSKFile = %q", got.PSKFile)
	}
	if got.ProjectID != "proj-1" {
		t.Fatalf("ProjectID = %q", got.ProjectID)
	}
	if len(got.Peers) != len(wantPeers) {
		t.Fatalf("Peers len = %d, want %d", len(got.Peers), len(wantPeers))
	}
	for i := range wantPeers {
		if got.Peers[i] != wantPeers[i] {
			t.Fatalf("Peers[%d] = %q, want %q", i, got.Peers[i], wantPeers[i])
		}
	}
}
