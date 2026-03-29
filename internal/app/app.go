// Package app provides the top-level application struct that wires together
// all subsystems. This is the foundation for eliminating the global Ctx
// singleton — instead of accessing core.Ctx directly, components receive
// the App (or its sub-managers) via dependency injection.
//
// Migration path:
//  1. New code should accept *App or specific managers as parameters
//  2. Existing code continues to use core.Ctx (backward compatible)
//  3. Gradually migrate each subsystem to use App instead of Ctx
//  4. Once all references are migrated, remove core.Ctx
package app

import (
	"github.com/WangYihang/Platypus/internal/listener"
	"github.com/WangYihang/Platypus/internal/session"
	"github.com/WangYihang/Platypus/internal/utils/config"
)

// App is the top-level application container that holds all managers
// and configuration. It replaces the global core.Ctx singleton.
type App struct {
	Config    *config.Config
	Sessions  *session.Manager
	Listeners *listener.Manager
}

// New creates a new App with initialized managers.
func New(cfg *config.Config) *App {
	return &App{
		Config:    cfg,
		Sessions:  session.NewManager(),
		Listeners: listener.NewManager(),
	}
}
