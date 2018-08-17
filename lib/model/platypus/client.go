package platypus

import (
	"net"
	"sync"
	"time"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/hash"
	"github.com/WangYihang/Platypus/lib/util/log"
)

type PlatypusClient struct {
	context.TCPClient
}

func CreatePlatypusClient(conn net.Conn) *PlatypusClient {
	return &PlatypusClient{
		context.TCPClient{
			TimeStamp:   time.Now(),
			Conn:        conn,
			Interactive: false,
			Group:       false,
			Hash:        hash.MD5(conn.RemoteAddr().String()),
			ReadLock:    new(sync.Mutex),
			WriteLock:   new(sync.Mutex),
		},
	}
}

func (c *PlatypusClient) Auth() bool {
	log.Info(">>>>>>>>>>>>>>>>>>><<<<<<<<<<<<<<<<<<<<")
	return false
}
