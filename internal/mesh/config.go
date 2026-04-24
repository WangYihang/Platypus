package mesh

import "crypto/x509"

// Config collects the runtime knobs for a mesh Node. Identity,
// TrustedCAs, and either PSK or PSKFile are required; the rest
// accept zero values.
type Config struct {
	// PSKFile is the pre-shared key path. Loaded lazily by
	// LoadOrCreatePSK if the caller didn't supply PSK directly.
	PSKFile string

	// PSK overrides PSKFile. 16+ bytes.
	PSK []byte

	// Identity is the cert-bound mesh identity (required). Callers
	// produce one via LoadIdentityFromCert.
	Identity *Identity

	// ListenAddr is the address the mesh listener binds to. If empty, no
	// inbound listener is started (the node can still dial out).
	// Use ":0" to let the OS pick a port (handy for tests).
	ListenAddr string

	// AdvertiseAddrs are the public-ish addresses this node advertises
	// for others to dial it on. If empty, ListenAddr is used verbatim.
	AdvertiseAddrs []string

	// Peers is the bootstrap peer list — plain "host:port" strings.
	// On startup the Dialer will try each one.
	Peers []string

	// Role is a human-readable tag (e.g. "agent", "server") carried in
	// logs. Does not affect protocol behaviour.
	Role string

	// DiscoveryLAN enables automatic peer discovery on the local network
	// via mDNS. Defaults to false (must be explicitly enabled).
	DiscoveryLAN bool

	// DiscoveryInterval is the time between mDNS browser refreshes.
	// Minimum 10s. Default 30s.
	DiscoveryInterval int

	// ProjectID is used in mDNS TXT records to isolate agents belonging
	// to different projects on the same LAN.
	ProjectID string

	// BootstrapEnabled marks this node as able to terminate bootstrap
	// streams for agents that cannot reach the server directly.
	BootstrapEnabled bool

	// BootstrapTarget is the address of the Platypus agent listener
	// (host:port) this node should dial when a bootstrap stream opens.
	// Meaningful only when BootstrapEnabled is true.
	BootstrapTarget string

	// TrustedCAs is the pool of CAs that will be used to verify
	// cert_pem fields on incoming gossip (MeshLSA, MeshPeerAnnounce,
	// MeshPeerDelta) and on incoming NodeInfo entries. When nil,
	// cert_pem fields on the wire are ignored and verification falls
	// back to the legacy DeriveNodeID(pubkey) self-cert check —
	// useful for pure-mesh tests that don't have a PKI.
	TrustedCAs *x509.CertPool
}
