package listeners

import "go.uber.org/zap"

// PlainShellListener represents a plain shell listener.
type PlainShellListener struct {
	Listener
}

// NewPlainShellListener creates a new plain shell listener.
func NewPlainShellListener(host string, port uint16) *PlainShellListener {
	return &PlainShellListener{
		Listener: Listener{
			BindHost: host,
			BindPort: port,
		},
	}
}

// Start starts the plain shell listener.
func (l *PlainShellListener) Start(logger *zap.Logger) {
	logger.Info("starting plain listener", zap.String("host", l.BindHost), zap.Uint16("port", l.BindPort))
}
