package reverse

import (
	"time"

	"github.com/WangYihang/Platypus/lib/context"
)

type ReverseServer struct {
	context.TCPServer
}

func CreateReverseServer(host string, port int16) *context.AbstractTCPServer {
	var abstractTCPServer context.AbstractTCPServer
	ts := time.Now()
	abstractTCPServer = &ReverseServer{
		context.TCPServer{
			Name:      "Reverse",
			Host:      host,
			Port:      port,
			Clients:   make(map[string](*context.TCPClient)),
			TimeStamp: ts,
		},
	}
	return &abstractTCPServer
}
