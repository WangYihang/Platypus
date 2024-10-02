package listeners

// ListenerType represents the type of listener.
type ListenerType string

const (
	// ListenerTypePlainShell represents a plain shell listener.
	ListenerTypePlainShell ListenerType = "plain_shell"
	// ListenerTypeEncryptedShell represents an encrypted shell listener.
	ListenerTypeEncryptedShell ListenerType = "encrypted_shell"
	// ListenerTypeRESTful represents a RESTful listener.
	ListenerTypeRESTful ListenerType = "restful"
)

// Listener is a struct that represents a listener
type Listener struct {
	BindHost string       `json:"bind_host" yaml:"bind_host" toml:"bind_host"`
	BindPort uint16       `json:"bind_port" yaml:"bind_port" toml:"bind_port"`
	Type     ListenerType `json:"type" yaml:"type" toml:"type"`
}
