package agent

import (
	"errors"
	"time"

	"github.com/WangYihang/Platypus/internal/mesh"
	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// errMeshRecvUnsupported is returned by meshCodec.Recv. The handler
// package only calls Recv from HandleConnection's TLS loop; mesh-backed
// clients receive envelopes via the Node's PayloadHandler callback
// instead, so Recv is never expected to be invoked on them.
var errMeshRecvUnsupported = errors.New("mesh codec does not support Recv")

// meshCodec adapts a mesh.Node's routed send path to the EnvelopeCodec
// interface. Outbound envelopes get SourceNode / TargetNode / TTL filled
// in by the mesh layer when SendTo is called.
type meshCodec struct {
	node *mesh.Node
	peer string
}

func newMeshCodec(node *mesh.Node, peer string) *meshCodec {
	return &meshCodec{node: node, peer: peer}
}

// Send forwards env toward the mesh peer this codec was built for. It
// clears any stale routing fields because the mesh layer is the single
// source of truth for them.
func (m *meshCodec) Send(env *agentpb.Envelope) error {
	if env == nil {
		return errors.New("nil envelope")
	}
	if env.Version == 0 {
		env.Version = protocolVersion
	}
	if env.Timestamp == 0 {
		env.Timestamp = time.Now().UnixNano()
	}
	return m.node.SendTo(m.peer, env)
}

// Recv is never called on a mesh codec — return a clear error so a bug
// is easier to diagnose.
func (m *meshCodec) Recv() (*agentpb.Envelope, error) {
	return nil, errMeshRecvUnsupported
}

// HandleMeshEnvelope dispatches an envelope that arrived through the
// mesh overlay and was addressed to this node. A synthetic Client is
// built so every response goes back via the same mesh peer.
func HandleMeshEnvelope(state *State, peer string, env *agentpb.Envelope) {
	if state == nil || state.Mesh == nil || env == nil {
		return
	}
	client := &Client{
		Codec:   newMeshCodec(state.Mesh, peer),
		Service: "mesh://" + peer,
	}
	dispatchEnvelope(client, state, env)
}
