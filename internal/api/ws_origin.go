package api

import (
	"os"

	"github.com/coder/websocket"
)

// wsAcceptOptions builds the websocket.AcceptOptions used by every
// browser-facing WebSocket upgrade in the server.
//
// Origin policy:
//
//   - Default (production): rely on coder/websocket's built-in
//     same-origin check (the request's Origin host must match the
//     request's Host header). Cross-origin WebSocket upgrades from
//     a malicious site are rejected with 403 — defense-in-depth on
//     top of the bearer-token authentication that already protects
//     these endpoints.
//
//   - Dev (PLATYPUS_DEV=1): the SPA runs on its own port (Vite at
//     :5173 by default) and the backend lives elsewhere
//     (:7332 / :9443), so a strict same-origin check would refuse
//     every browser request from the dev server. Set
//     InsecureSkipVerify to relax the check during development.
//
// Subprotocols are appended verbatim. The auth subprotocol path
// (Sec-WebSocket-Protocol: Bearer.<token>) is handled at the handler
// level — see RequireAuthWS in rbac.go — so this helper doesn't need
// to know about it.
func wsAcceptOptions(subprotocols ...string) *websocket.AcceptOptions {
	return &websocket.AcceptOptions{
		Subprotocols:       subprotocols,
		InsecureSkipVerify: isDevMode(),
	}
}

// isDevMode reports whether PLATYPUS_DEV is the truthy value "1".
// Mirrors the check used in cmd/platypus-server/main.go for the KEK
// fallback so a single env var controls every dev-only relaxation
// the server applies.
func isDevMode() bool {
	return os.Getenv("PLATYPUS_DEV") == "1"
}
