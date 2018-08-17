package platypus

import (
	"fmt"
	"time"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/hash"
)

type PlatypusTCPServer struct {
	context.TCPServer
}

func CreatePlatypusTCPServer(host string, port int16) *PlatypusTCPServer {
	ts := time.Now()
	return &PlatypusTCPServer{
		context.TCPServer{
			Name:      "Reverse",
			Host:      host,
			Port:      port,
			Clients:   make(map[string](*context.Client)),
			TimeStamp: ts,
			Hash:      hash.MD5(fmt.Sprintf("%s:%s:%s", host, port, ts)),
		},
	}
}
