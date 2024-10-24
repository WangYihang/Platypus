package listeners

import "go.uber.org/zap"

// EncryptedListener represents an encrypted shell listener.
type EncryptedListener struct {
	commonListener
}

// NewEncryptedListener creates a new encrypted shell listener.
func NewEncryptedListener(host string, port uint16) *EncryptedListener {
	return &EncryptedListener{
		commonListener: commonListener{
			BindHost: host,
			BindPort: port,
		},
	}
}

// Start starts the encrypted shell listener.
func (l *EncryptedListener) Start(logger *zap.Logger) {
	logger.Info("starting encrypted listener", zap.String("host", l.BindHost), zap.Uint16("port", l.BindPort))
}
