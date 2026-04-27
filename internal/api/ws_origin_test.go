package api

import (
	"testing"
)

// L1: the previous WebSocket upgrade in handler_terminal_v2 set
// InsecureSkipVerify:true unconditionally. That disabled
// coder/websocket's built-in same-origin check on every browser
// upgrade — defence-in-depth gone, even though authn was already
// bearer-token-based. The audit's call was to gate the relaxation
// behind PLATYPUS_DEV so production deployments enforce same-origin
// while local dev (Vite at :5173 + backend at :7332) keeps working.

func TestWSAcceptOptions_StrictByDefault(t *testing.T) {
	t.Setenv("PLATYPUS_DEV", "")
	opts := wsAcceptOptions("tty")
	if opts.InsecureSkipVerify {
		t.Fatal(
			"wsAcceptOptions returned InsecureSkipVerify=true with PLATYPUS_DEV unset; " +
				"production must enforce same-origin",
		)
	}
	if len(opts.Subprotocols) != 1 || opts.Subprotocols[0] != "tty" {
		t.Errorf("Subprotocols not propagated: got %v, want [tty]", opts.Subprotocols)
	}
}

func TestWSAcceptOptions_DevModeRelaxes(t *testing.T) {
	t.Setenv("PLATYPUS_DEV", "1")
	opts := wsAcceptOptions("tty")
	if !opts.InsecureSkipVerify {
		t.Fatal(
			"wsAcceptOptions returned InsecureSkipVerify=false with PLATYPUS_DEV=1; " +
				"the dev-mode relaxation didn't fire",
		)
	}
}

// PLATYPUS_DEV must be the literal "1" — anything else (even truthy
// strings like "true" / "yes") falls through to strict mode. Pinning
// the env-var contract avoids subtle "I set PLATYPUS_DEV=anything and
// it broke" reports.
func TestWSAcceptOptions_DevModeOnlyOnExactOne(t *testing.T) {
	t.Setenv("PLATYPUS_DEV", "true")
	opts := wsAcceptOptions("tty")
	if opts.InsecureSkipVerify {
		t.Fatal("PLATYPUS_DEV='true' must NOT enable dev-mode; only the literal \"1\" does")
	}
}
