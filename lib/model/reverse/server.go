package reverse

import (
	"fmt"
	"time"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/hash"
)

type ReverseServer struct {
	context.TCPServer
}

func CreateReverseServer(host string, port int16) *ReverseServer {
	ts := time.Now()
	return &ReverseServer{
		context.TCPServer{
			Name:      "Reverse",
			Host:      host,
			Port:      port,
			Clients:   make(map[string](*context.TCPClient)),
			TimeStamp: ts,
			Hash:      hash.MD5(fmt.Sprintf("%s:%s:%s", host, port, ts)),
		},
	}
}
