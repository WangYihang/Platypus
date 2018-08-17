package platypus

import (
	"net"
	"sync"
	"time"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/crypto"
	"github.com/WangYihang/Platypus/lib/util/hash"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/str"
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
	key := "VnwkyMTUgmzVxUi6"
	tokenLenth := 0x100
	token := str.RandomString(tokenLenth)
	cipher, err := crypto.Encrypt([]byte(key), token)
	if err != nil {
		log.Info("Encrypting token failed: %s", err)
		return false
	}
	c.Write([]byte(cipher))
	answer := c.ReadSize(tokenLenth)
	if answer != token {
		return false
	}
	return true
}
