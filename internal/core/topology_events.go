// Package core — Topology link event observer.
//
// InstallTopologyObserver registers a mesh.LinkObserver that emits
// topology.link_up / topology.link_down frames through BroadcastNotify
// whenever the server's own mesh Node sees adjacency change. The
// observer fans out to every project currently touching the mesh —
// same semantics as the 1Hz stats broadcaster in topology_stream.go.
package core

import (
	"time"

	"github.com/WangYihang/Platypus/internal/mesh"
)

// linkEventObserver is the concrete observer we register with
// mesh.Node. Holds no state beyond what BroadcastNotify needs.
type linkEventObserver struct{}

func (linkEventObserver) OnLinkUp(peerNodeID, remoteAddr string) {
	for projectID := range activeProjectSet() {
		BroadcastNotify(EventTopologyLinkUp, map[string]interface{}{
			"project_id":  projectID,
			"peer":        peerNodeID,
			"remote_addr": remoteAddr,
			"at":          time.Now().UTC(),
		})
	}
}

func (linkEventObserver) OnLinkDown(peerNodeID string) {
	for projectID := range activeProjectSet() {
		BroadcastNotify(EventTopologyLinkDown, map[string]interface{}{
			"project_id": projectID,
			"peer":       peerNodeID,
			"at":         time.Now().UTC(),
		})
	}
}

// InstallTopologyObserver registers the link observer on the server's
// mesh node. No-op when mesh is disabled. Returns a deregister func
// the caller can defer.
func InstallTopologyObserver() func() {
	if Ctx == nil {
		return func() {}
	}
	node, ok := Ctx.Mesh.(*mesh.Node)
	if !ok || node == nil {
		return func() {}
	}
	return node.RegisterLinkObserver(linkEventObserver{})
}

// activeProjectSet returns the set of projectIDs that currently have
// at least one connected agent. Used by the link event observer and
// the 1 Hz stats loop to decide which projects receive a broadcast.
func activeProjectSet() map[string]struct{} {
	out := map[string]struct{}{}
	for _, ac := range allAgentClients() {
		if ac == nil || ac.ProjectID == "" {
			continue
		}
		out[ac.ProjectID] = struct{}{}
	}
	return out
}
