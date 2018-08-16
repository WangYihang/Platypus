package reverse

import (
	"github.com/WangYihang/Platypus/lib/context"
)

type ReverseServer struct {
	TCPServer *context.Server
}

func CreateReverseServer(host string, port int16) *ReverseServer {
	server := &ReverseServer{
		TCPServer: context.CreateServer(host, port),
	}
	return server
}
