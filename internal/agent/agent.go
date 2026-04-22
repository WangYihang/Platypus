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

// ConnectOptions carries knobs that Connect doesn't otherwise receive
// directly. Nil-safe (callers on the legacy path pass nil and get the
// old behaviour).
type ConnectOptions struct {
	// IdentityDir is where the agent persists its session_token. When
	// empty, defaults to ~/.platypus/agent.
	IdentityDir string
}

// Connect establishes a TLS connection to the server endpoint and runs
// the message handler loop.
func Connect(endpoint, token string, state *State) error {
	return ConnectWithOptions(endpoint, token, state, nil)
}

// ConnectWithOptions is Connect + optional enrollment knobs.
func ConnectWithOptions(endpoint, token string, state *State, opts *ConnectOptions) error {
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

		// Optional enrollment handshake. Runs when we have a PAT or a
		// previously persisted session token. On reject we abort (a
		// bad/revoked credential must not silently fall back to the
		// legacy unauthenticated path). On absence, we continue straight
		// into HandleConnection — legacy compatibility.
		identityDir := ""
		if opts != nil {
			identityDir = opts.IdentityDir
		}
		er, err := MaybeEnroll(c, token, identityDir)
		if err != nil {
			log.Error("Enrollment error: %s", err)
			return err
		}
		if er.Attempted && !er.Succeeded {
			return fmt.Errorf("agent: enrollment rejected: %s", er.ErrorMessage)
		}
		if er.Succeeded {
			log.Success("Enrolled with server (agent_id=%s, session expires %s)",
				er.AgentID, er.SessionExpiresAt.Format("2006-01-02 15:04:05"))
		}

		// Kick off the in-band rotation goroutine. It schedules itself
		// RenewGrace before expiry and keeps going until the stop
		// channel closes (on normal return from HandleConnection).
		// Skipped when enrollment didn't actually happen so legacy
		// agents don't try to rotate a non-existent token.
		stopRenew := make(chan struct{})
		if er.Succeeded {
			StartRenewalLoop(RenewalContext{
				Client:       c,
				IdentityDir:  identityDir,
				CurrentToken: er.SessionToken,
				ExpiresAt:    er.SessionExpiresAt,
			}, stopRenew)
		}
		defer close(stopRenew)

		HandleConnection(c, state)
		return nil
	}
	return err
}

// Init initializes the agent's state.
func Init() *State {
	return NewState()
}
