package session

import (
	"net"
	"time"

	oss "github.com/WangYihang/Platypus/internal/utils/os"
)

// Session represents a connected agent managed by the server.
type Session interface {
	// Identity
	GetHash() string
	GetAlias() string
	SetAlias(alias string)

	// Connection info
	GetHost() string
	GetPort() uint16
	GetConn() net.Conn
	GetConnString() string

	// Client metadata
	GetUsername() string
	GetOS() oss.OperatingSystem
	GetTimeStamp() time.Time
	GetGroupDispatch() bool
	SetGroupDispatch(v bool)

	// Display
	OnelineDesc() string
	FullDesc() string

	// Operations
	Execute(command string) (string, error)

	// Lifecycle
	Close()
}
