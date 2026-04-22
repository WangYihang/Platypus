package agent

import (
	"crypto/tls"
	"fmt"
	"net"
	"strings"

	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/mesh"
	"github.com/WangYihang/Platypus/internal/protocol"
	"github.com/WangYihang/Platypus/internal/utils/crypto"
	"github.com/WangYihang/Platypus/internal/utils/hash"
	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// EnvelopeCodec is the minimal interface handler.go needs from whatever
// transports envelopes — a TLS connection to the server or a mesh-backed
// adapter that routes through the overlay.
type EnvelopeCodec interface {
	Send(env *agentpb.Envelope) error
	Recv() (*agentpb.Envelope, error)
}

// Compile-time check: *protocol.ProtoCodec still satisfies our interface.
var _ EnvelopeCodec = (*protocol.ProtoCodec)(nil)

// Client represents an agent's connection to the platypus server or to
// a mesh peer. When a Client is backed by the mesh, Conn is nil and
// Codec is a mesh adapter.
type Client struct {
	Conn    *tls.Conn
	Codec   EnvelopeCodec
	Service string
}

// State holds the mutable state for a running agent.
type State struct {
	Processes      *ProcessMap
	PullTunnels    *ConnMap
	PushTunnels    *ConnMap
	Socks5Listener *net.Listener

	// Mesh is set by AttachMesh when the agent is running with the
	// overlay enabled. Nil for legacy hub-and-spoke deployments.
	Mesh *mesh.Node
}

// NewState creates a new initialized agent state.
func NewState() *State {
	return &State{
		Processes:   NewProcessMap(),
		PullTunnels: NewConnMap(),
		PushTunnels: NewConnMap(),
	}
}

// AttachMesh wires a mesh.Node into the agent state and installs a
// payload handler so envelopes targeted at this node get dispatched
// through the normal agent handler path (HandleMeshEnvelope).
func AttachMesh(state *State, node *mesh.Node) {
	state.Mesh = node
	node.SetPayloadHandler(func(peer string, env *agentpb.Envelope) {
		HandleMeshEnvelope(state, peer, env)
	})
}

// SendEnvelope sends a protobuf envelope via the codec.
func (c *Client) SendEnvelope(env *agentpb.Envelope) error {
	return c.Codec.Send(env)
}

// RecvEnvelope receives a protobuf envelope via the codec.
func (c *Client) RecvEnvelope() (*agentpb.Envelope, error) {
	return c.Codec.Recv()
}

// Connect establishes a TLS connection to the server endpoint and runs
// the message handler loop.
func Connect(endpoint, token string, state *State) error {
	certBuilder := new(strings.Builder)
	keyBuilder := new(strings.Builder)
	crypto.Generate(certBuilder, keyBuilder)

	pemContent := []byte(fmt.Sprint(certBuilder))
	keyContent := []byte(fmt.Sprint(keyBuilder))

	cert, err := tls.X509KeyPair(pemContent, keyContent)
	if err != nil {
		log.Error("server: loadkeys: %s", err)
		return err
	}

	config := tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true}
	if hash.MD5(endpoint) != "4d1bf9fd5962f16f6b4b53a387a6d852" { // pragma: allowlist secret
		log.Debug("Connecting to: %s", endpoint)
		conn, err := tls.Dial("tcp", endpoint, &config)
		if err != nil {
			log.Error("client: dial: %s", err)
			return err
		}
		defer conn.Close()

		log.Success("Secure connection established on %s", conn.RemoteAddr())

		c := &Client{
			Conn:    conn,
			Codec:   protocol.NewProtoCodec(conn),
			Service: endpoint,
		}
		HandleConnection(c, state)
		return nil
	}
	return err
}

// Init initializes the agent's state.
func Init() *State {
	return NewState()
}
