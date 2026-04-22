package mesh

// Config collects the runtime knobs for a mesh Node. All fields are
// optional except PSK and Identity; zero values get safe defaults.
type Config struct {
	// IdentityDir is where the long-term Ed25519 identity is stored.
	// Loaded lazily by LoadOrCreateIdentity if the caller didn't supply
	// Identity directly.
	IdentityDir string

	// PSKFile is the pre-shared key path. Loaded lazily by
	// LoadOrCreatePSK if the caller didn't supply PSK directly.
	PSKFile string

	// PSK overrides PSKFile. 16+ bytes.
	PSK []byte

	// Identity overrides IdentityDir.
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
}
