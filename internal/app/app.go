// Package app provides the top-level application struct that wires together
// all subsystems, replacing the former global core.Ctx singleton.
package app

import (
	"net"
	"os"
	"strings"
	"sync"

	"github.com/WangYihang/Platypus/internal/listener"
	"github.com/WangYihang/Platypus/internal/session"
	"github.com/WangYihang/Platypus/internal/utils/config"
	"github.com/WangYihang/Platypus/internal/utils/message"
	"github.com/WangYihang/Platypus/internal/utils/ui"
	"github.com/WangYihang/readline"
	"github.com/armon/go-socks5"
	"github.com/fatih/color"
	"github.com/gin-gonic/gin"
	"gopkg.in/olahol/melody.v1"
)

// PullTunnelConfig represents a local-to-remote port forwarding configuration.
type PullTunnelConfig struct {
	Termite interface{} // *core.TermiteClient (avoids circular import)
	Address string
	Server  *net.Listener
}

// PullTunnelInstance represents an active pull tunnel connection.
type PullTunnelInstance struct {
	Termite interface{} // *core.TermiteClient
	Conn    *net.Conn
}

// PushTunnelConfig represents a remote-to-local port forwarding configuration.
type PushTunnelConfig struct {
	Termite interface{} // *core.TermiteClient
	Address string
}

// PushTunnelInstance represents an active push tunnel connection.
type PushTunnelInstance struct {
	Termite interface{} // *core.TermiteClient
	Conn    *net.Conn
}

// App is the top-level application container.
type App struct {
	Config          *config.Config
	Sessions        *session.Manager
	Listeners       *listener.Manager

	// Current session state
	Current        interface{} // *core.TCPClient (avoids circular import)
	CurrentTermite interface{} // *core.TermiteClient

	// Server registry (keyed by hash)
	Servers map[string]interface{} // map[string]*core.TCPServer

	// UI
	CommandPrompt string
	RLInstance    *readline.Instance
	Interacting   *sync.Mutex

	// WebSocket
	NotifyWebSocket *melody.Melody

	// Tunneling
	PullTunnelConfig   map[string]PullTunnelConfig
	PullTunnelInstance map[string]PullTunnelInstance
	PushTunnelConfig   map[string]PushTunnelConfig
	PushTunnelInstance map[string]PushTunnelInstance
	Socks5Servers      map[string]*socks5.Server

	// Messaging
	MessageQueue   map[string](chan message.Message)
	MessageQueueMu sync.RWMutex

	// Distributor
	Distributor interface{} // *core.Distributor

	// REST
	RESTful *gin.Engine
}

// New creates a new App with initialized managers.
func New(cfg *config.Config) *App {
	return &App{
		Config:             cfg,
		Sessions:           session.NewManager(),
		Listeners:          listener.NewManager(),
		Servers:            make(map[string]interface{}),
		CommandPrompt:      color.CyanString("» "),
		Interacting:        &sync.Mutex{},
		PullTunnelConfig:   make(map[string]PullTunnelConfig),
		PullTunnelInstance: make(map[string]PullTunnelInstance),
		PushTunnelConfig:   make(map[string]PushTunnelConfig),
		PushTunnelInstance: make(map[string]PushTunnelInstance),
		Socks5Servers:      make(map[string]*socks5.Server),
		MessageQueue:       make(map[string](chan message.Message)),
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

// SetCurrentSession sets the current interactive session and updates the prompt.
func (a *App) SetCurrentSession(s session.Session) {
	a.Current = nil
	a.CurrentTermite = nil
	if a.RLInstance != nil && s != nil {
		a.RLInstance.SetPrompt(color.CyanString(s.GetPrompt()))
	}
}

// AllSessions returns all sessions from all listeners.
func (a *App) AllSessions() []session.Session {
	return a.Sessions.All()
}

// FindListener searches listeners by hash prefix.
func (a *App) FindListener(clue string) listener.Listener {
	if clue == "" {
		return nil
	}
	return a.Listeners.FindByHashPrefix(strings.ToLower(clue))
}

// Shutdown gracefully stops all servers and exits.
func (a *App) Shutdown() {
	if len(a.Servers) > 0 && !ui.PromptYesNo("There are listening servers, do you really want to exit?") {
		return
	}
	os.Exit(0)
}
