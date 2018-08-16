package reverse

import (
	"github.com/WangYihang/Platypus/lib/context"
)

type ReverseTCPServer interface {
	context.TCPServer
}

type BaseReverseTCPServer struct {
	context.BaseTCPServer
}

func CreateReverseServer(host string, port int16) *BaseReverseTCPServer {
	return &BaseReverseTCPServer{}
}
