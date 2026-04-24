// Package ingress multiplexes every inbound connection Platypus cares
// about — agent protobuf frames, mesh overlay, HTTPS admin REST + WS,
// and the bootstrap install/artifact downloads — onto a single TLS
// port using ALPN to pick the right handler.
//
// The server advertises every supported protocol in its tls.Config's
// NextProtos; the client picks one during the TLS handshake. After
// the handshake, Dispatcher looks at ConnectionState.NegotiatedProtocol
// and hands the post-TLS net.Conn to the matching callback.
package ingress

// ALPN protocol identifiers. Kept short: they ride on every TLS
// ClientHello, and a short token is enough to disambiguate. The
// "ptps-" prefix reserves a Platypus namespace that won't collide
// with standard IANA-registered protocols (h2, http/1.1, etc.).
//
// Bumping a protocol's wire format means adding a new constant
// (e.g. "ptps-agent-2") rather than reinterpreting the old one —
// old and new clients coexist during the rollout.
const (
	// ALPNAgent is negotiated by platypus-agent binaries connecting
	// back to the server to deliver a protobuf Envelope stream.
	ALPNAgent = "ptps-agent"

	// ALPNMesh is negotiated between mesh peers, server-to-agent or
	// agent-to-agent. The post-TLS stream runs MeshHello / MeshHelloAck
	// before any routing payload.
	ALPNMesh = "ptps-mesh"

	// ALPNHTTP2 / ALPNHTTP1 are the standard IANA identifiers for
	// HTTP. Anything negotiating these ends up in the shared gin
	// router (API v1, WebSocket endpoints, distributor).
	ALPNHTTP2 = "h2"
	ALPNHTTP1 = "http/1.1"
)

// DefaultProtocols is the canonical NextProtos slice to advertise on
// the server side. Order matters: if multiple entries overlap with
// the client's offer the server picks the first match, so keep the
// more-specific Platypus protocols before the generic HTTP ones.
var DefaultProtocols = []string{
	ALPNAgent,
	ALPNMesh,
	ALPNHTTP2,
	ALPNHTTP1,
}
