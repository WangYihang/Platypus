package core

import (
	"github.com/WangYihang/Platypus/internal/app"
	"github.com/WangYihang/Platypus/internal/enrollment"
)

// Ctx is the process-wide application container, set by main()
// exactly once via CreateContext(). Callers that need the
// database, session registry, or notify fan-out reach for this.
//
// The v1 agent machinery that used to hang off Ctx (AgentClient
// maps, EnvelopeQueue, etc.) is gone; Ctx survives only because
// the distributor + some REST handlers still stash cross-cutting
// state here rather than threading it through every call.
var Ctx *app.App

// CreateContext is the one-time initialiser main() calls during
// bootstrap. No-op today — App.New already returns a live object;
// this entry point stays so call sites read cleanly and because
// future wiring (lifecycle hooks, backgrounding jobs) will need a
// place to land.
func CreateContext() {}

// enrollSvc is the singleton enrollment.Service used by the
// distributor's install-token → PAT hand-off. Set once by
// SetEnrollment during bootstrap.
var enrollSvc *enrollment.Service

// SetEnrollment wires the enrollment service into this package.
// Safe to call before any request hits the distributor because
// main() runs both before starting the HTTP server.
func SetEnrollment(svc *enrollment.Service) { enrollSvc = svc }
