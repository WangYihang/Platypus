package session

import (
	"net"
	"time"

	oss "github.com/WangYihang/Platypus/internal/utils/os"
)

// Session represents a reverse shell session, either plain TCP or encrypted Termite.
type Session interface {
	// Identity
	GetHash() string
	GetAlias() string
	SetAlias(alias string)
	IsEncrypted() bool

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
	GetPrompt() string
	OnelineDesc() string
	FullDesc() string
	AsTable()

	// Operations
	Execute(command string) (string, error)

	// Lifecycle
	Close()
}
