package core

import (
	"net"

	"github.com/WangYihang/Platypus/internal/protocol"
)

// NewAgentClientForTest builds a minimal AgentClient around a net.Conn
// for tests that want to exercise enrollment / handshake code without
// spinning up a full TLS TCPServer. Exported only via _test.go so
// production callers can't accidentally skip CreateAgentClient.
func NewAgentClientForTest(conn net.Conn) *AgentClient {
	return &AgentClient{
		conn:  conn,
		codec: protocol.NewProtoCodec(conn),
	}
}
