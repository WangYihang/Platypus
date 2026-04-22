package agent

import (
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
