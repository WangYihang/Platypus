// Package agent holds the code that runs on a managed host under
// the platypus-agent binary. The v1 envelope protocol and its
// handler machinery are gone; the current bring-up path is
// BootstrapV2 (see bootstrap_v2.go) which produces a live
// *link.Session the agent drives with ServeLink + the registered
// per-stream handlers.
//
// State survives — with a much thinner shape — to hold mesh-bootstrap
// material that outlives a single connect attempt.
package agent

import (
	"github.com/WangYihang/Platypus/internal/mesh"
)

// State is the per-agent mutable state retained across reconnect
// attempts. After the v1 deletion pass it carries only the mesh
// node pointer; v1's process / tunnel / socks5 maps are dead.
type State struct {
	// Mesh is set by AttachMesh when the agent is running with the
	// overlay enabled. Nil for plain hub-and-spoke deployments.
	Mesh *mesh.Node
}

// Init returns a fresh State. Kept as a package-level helper so
// callers (cmd/platypus-agent/main.go) don't have to reach for
// struct-literal construction of a struct that may gain fields
// again later.
func Init() *State { return &State{} }

// AttachMesh associates a started *mesh.Node with the agent state.
// Mesh stream dispatch is a Phase IV concern — for now we just hold
// the reference so the agent main loop can report liveness; the
// overlay's payload-handler hook is left nil, which means envelopes
// destined for this node that arrive via mesh are dropped.
func AttachMesh(state *State, node *mesh.Node) {
	state.Mesh = node
}
