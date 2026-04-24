// Package app provides the top-level application struct that wires together
// all subsystems, replacing the former global core.Ctx singleton.
package app

import (
	"net"
	"sync"

	"github.com/gin-gonic/gin"
	socks5 "github.com/things-go/go-socks5"
	"gopkg.in/olahol/melody.v1"

	"github.com/WangYihang/Platypus/internal/session"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/utils/config"
)

// PullTunnelConfig represents a local-to-remote port forwarding configuration.
type PullTunnelConfig struct {
	Agent   interface{} // *core.AgentClient (avoids circular import)
	Address string
	Server  *net.Listener
}

// PullTunnelInstance represents an active pull tunnel connection.
type PullTunnelInstance struct {
	Agent interface{} // *core.AgentClient
	Conn  *net.Conn
}

// PushTunnelConfig represents a remote-to-local port forwarding configuration.
type PushTunnelConfig struct {
	Agent   interface{} // *core.AgentClient
	Address string
}

// PushTunnelInstance represents an active push tunnel connection.
type PushTunnelInstance struct {
	Agent interface{} // *core.AgentClient
	Conn  *net.Conn
}

// App is the top-level application container.
type App struct {
	Config   *config.Config
	Sessions *session.Manager

	// Current session state
	CurrentAgent interface{} // *core.AgentClient

	// Concurrency
	Interacting *sync.Mutex

	// WebSocket
	NotifyWebSocket *melody.Melody

	// Tunneling
	PullTunnelConfig   map[string]PullTunnelConfig
	PullTunnelInstance map[string]PullTunnelInstance
	PushTunnelConfig   map[string]PushTunnelConfig
	PushTunnelInstance map[string]PushTunnelInstance
	Socks5Servers      map[string]*socks5.Server

	// Messaging (protobuf RPC response channels)
	EnvelopeQueue  map[string](chan interface{})
	MessageQueueMu sync.RWMutex

	// Distributor
	Distributor interface{} // *core.Distributor

	// REST
	RESTful *gin.Engine

	// Storage holds the SQLite-backed persistence layer for users,
	// projects, hosts, and sessions. Nil when the RESTful subsystem is
	// disabled in config (older deployments that never wanted HTTP).
	Storage *storage.DB

	// Mesh holds the overlay node when the server opts into the agent
	// mesh (config.Mesh.PSKFile set). Typed as interface{} to avoid a
	// cycle with core/*; the concrete type is *mesh.Node. Nil means
	// legacy hub-and-spoke mode.
	Mesh interface{}
}

// New creates a new App with initialized managers.
func New(cfg *config.Config) *App {
	return &App{
		Config:             cfg,
		Sessions:           session.NewManager(),
		Interacting:        &sync.Mutex{},
		PullTunnelConfig:   make(map[string]PullTunnelConfig),
		PullTunnelInstance: make(map[string]PullTunnelInstance),
		PushTunnelConfig:   make(map[string]PushTunnelConfig),
		PushTunnelInstance: make(map[string]PushTunnelInstance),
		Socks5Servers:      make(map[string]*socks5.Server),
		EnvelopeQueue:      make(map[string](chan interface{})),
	}
}

// FindSession searches all sessions by hash prefix, then by alias prefix.
func (a *App) FindSession(clue string) session.Session {
	if clue == "" {
		return nil
	}
	if s := a.Sessions.FindByHashPrefix(clue); s != nil {
		return s
	}
	return a.Sessions.FindByAlias(clue)
}

// SetCurrentSession sets the current interactive session.
func (a *App) SetCurrentSession(s session.Session) {
	a.CurrentAgent = nil
}

// AllSessions returns all sessions from all listeners.
func (a *App) AllSessions() []session.Session {
	return a.Sessions.All()
}
