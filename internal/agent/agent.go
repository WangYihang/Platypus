package agent

import (
	"crypto/tls"
	"fmt"
	"net"
	"strings"

	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/protocol"
	"github.com/WangYihang/Platypus/internal/utils/crypto"
	"github.com/WangYihang/Platypus/internal/utils/hash"
	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// Client represents an agent's connection to the platypus server.
type Client struct {
	Conn    *tls.Conn
	Codec   *protocol.ProtoCodec
	Service string
}

// State holds the mutable state for a running agent.
type State struct {
	Processes      *ProcessMap
	PullTunnels    *ConnMap
	PushTunnels    *ConnMap
	Socks5Listener *net.Listener
}

// NewState creates a new initialized agent state.
func NewState() *State {
	return &State{
		Processes:   NewProcessMap(),
		PullTunnels: NewConnMap(),
		PushTunnels: NewConnMap(),
	}
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
