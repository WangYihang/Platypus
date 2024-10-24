package listeners

import "go.uber.org/zap"

// PlainListener represents a plain shell listener.
type PlainListener struct {
	commonListener
}

// NewPlainListener creates a new plain shell listener.
func NewPlainListener(host string, port uint16) *PlainListener {
	return &PlainListener{
		commonListener: commonListener{
			BindHost: host,
			BindPort: port,
		},
	}
}

// Start starts the plain shell listener.
func (l *PlainListener) Start(logger *zap.Logger) {
	logger.Info("starting plain listener", zap.String("host", l.BindHost), zap.Uint16("port", l.BindPort))
}
