package listener

// Listener represents a TLS ingress that accepts agent connections.
type Listener interface {
	// Identity
	GetHash() string
	GetHost() string
	GetPort() uint16

	// Lifecycle
	Run()
	Stop()
}
