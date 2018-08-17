package reverse

import (
	"fmt"
	"time"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/hash"
)

type ReverseTCPServer struct {
	context.TCPServer
}

func CreateReverseTCPServer(host string, port int16) *ReverseTCPServer {
	ts := time.Now()
	return &ReverseTCPServer{
		context.TCPServer{
			Host:      host,
			Port:      port,
			Clients:   make(map[string](*context.Client)),
			TimeStamp: ts,
			Hash:      hash.MD5(fmt.Sprintf("%s:%s:%s", host, port, ts)),
		},
	}
}
