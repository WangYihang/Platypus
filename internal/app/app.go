// Package app provides the top-level application container that
// wires together the subsystems the server keeps live for the
// duration of its run. With the v1 Envelope protocol deleted the
// container is a lot thinner — it's now the config + session
// registry + notify fan-out + storage + mesh node.
package app

import (
	"github.com/gin-gonic/gin"
	"gopkg.in/olahol/melody.v1"

	"github.com/WangYihang/Platypus/internal/session"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/utils/config"
)

// App is the top-level application container. Fields that only the
// v1 codepath populated (tunnel maps, EnvelopeQueue, CurrentAgent,
// etc.) have been removed — their jobs live on v2 types now
// (core.AgentLinkService, core.TunnelService once it lands, etc.).
type App struct {
	Config   *config.Config
	Sessions *session.Manager

	// NotifyWebSocket fans topology / lifecycle events out to the
	// browser UI in /notify.
	NotifyWebSocket *melody.Melody

	// Distributor is the object-store-backed agent binary server.
	// Typed as interface{} to keep app free of a core import cycle.
	Distributor interface{} // *core.Distributor

	RESTful *gin.Engine

	// Storage is the SQLite-backed persistence layer. Nil when the
	// RESTful subsystem is disabled in config.
	Storage *storage.DB

	// Mesh is the overlay node when config.Mesh.PSKFile is set.
	// Typed as interface{} to avoid a cycle with core/*; the
	// concrete type is *mesh.Node.
	Mesh interface{}
}

// New creates a fresh App with initialised managers.
func New(cfg *config.Config) *App {
	return &App{
		Config:   cfg,
		Sessions: session.NewManager(),
	}
}

// FindSession searches all sessions by hash prefix, then by alias
// prefix. Used by the few admin UI handlers that still look up
// sessions by an opaque identifier.
func (a *App) FindSession(clue string) session.Session {
	if clue == "" {
		return nil
	}
	if s := a.Sessions.FindByHashPrefix(clue); s != nil {
		return s
	}
	return a.Sessions.FindByAlias(clue)
}

// AllSessions returns a snapshot of every registered session.
func (a *App) AllSessions() []session.Session {
	return a.Sessions.All()
}
