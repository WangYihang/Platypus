// Package app provides the top-level application struct that wires together
// all subsystems. It replaces the global core.Ctx singleton with explicit
// dependency injection.
package app

import (
	"strings"
	"sync"

	"github.com/WangYihang/Platypus/internal/listener"
	"github.com/WangYihang/Platypus/internal/session"
	"github.com/WangYihang/Platypus/internal/utils/config"
	"github.com/WangYihang/Platypus/internal/utils/message"
	"github.com/WangYihang/readline"
	"github.com/armon/go-socks5"
	"github.com/gin-gonic/gin"
	"gopkg.in/olahol/melody.v1"
)

// App is the top-level application container.
type App struct {
	Config          *config.Config
	Sessions        *session.Manager
	Listeners       *listener.Manager
	CurrentSession  session.Session
	Prompt          *readline.Instance
	Interacting     *sync.Mutex
	NotifyWebSocket *melody.Melody
	MessageQueue    map[string](chan message.Message)
	MessageQueueMu  sync.RWMutex
	Socks5Servers   map[string]*socks5.Server
	RESTful         *gin.Engine
}

// New creates a new App with initialized managers.
func New(cfg *config.Config) *App {
	return &App{
		Config:        cfg,
		Sessions:      session.NewManager(),
		Listeners:     listener.NewManager(),
		Interacting:   &sync.Mutex{},
		MessageQueue:  make(map[string](chan message.Message)),
		Socks5Servers: make(map[string]*socks5.Server),
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
	a.CurrentSession = s
	if a.Prompt != nil && s != nil {
		a.Prompt.SetPrompt(s.GetPrompt())
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
