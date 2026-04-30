package mesh

import (
	"context"
	"net"

	"github.com/coder/websocket"
)

// LinkSubprotocol is the Sec-WebSocket-Protocol value both sides of a
// /api/v1/mesh/link upgrade negotiate. Distinct from the agent link
// subprotocol so a misconfigured agent can't accidentally talk to the
// mesh handler (and vice versa).
const LinkSubprotocol = "ptps-mesh.v1"

// NetConnFromWebSocket wraps a *websocket.Conn so envCodec can use it
// unchanged. MessageBinary because protobuf bytes aren't UTF-8.
func NetConnFromWebSocket(ctx context.Context, ws *websocket.Conn) net.Conn {
	return websocket.NetConn(ctx, ws, websocket.MessageBinary)
}
