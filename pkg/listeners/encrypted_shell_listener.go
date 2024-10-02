package listeners

import "go.uber.org/zap"

// EncryptedShellListener represents an encrypted shell listener.
type EncryptedShellListener struct {
	Listener
}

// NewEncryptedShellListener creates a new encrypted shell listener.
func NewEncryptedShellListener(host string, port uint16) *EncryptedShellListener {
	return &EncryptedShellListener{
		Listener: Listener{
			BindHost: host,
			BindPort: port,
		},
	}
}

// Start starts the encrypted shell listener.
func (l *EncryptedShellListener) Start(logger *zap.Logger) {
	logger.Info("starting encrypted listener", zap.String("host", l.BindHost), zap.Uint16("port", l.BindPort))
}
