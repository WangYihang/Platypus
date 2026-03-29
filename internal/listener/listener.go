package listener

// Listener represents a network listener that accepts reverse shell connections.
type Listener interface {
	// Identity
	GetHash() string
	GetHost() string
	GetPort() uint16
	IsEncrypted() bool

	// Display
	FullDesc() string

	// Lifecycle
	Run()
	Stop()
}
