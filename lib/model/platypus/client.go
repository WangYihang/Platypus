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
	log.Info("Auth process started")
	key := "VnwkyMTUgmzVxUi6"
	tokenLenth := 0x100
	token := str.RandomString(tokenLenth)
	log.Info("Token: %s", token)
	cipher, err := crypto.Encrypt([]byte(key), token)
	if err != nil {
		log.Info("Encrypting token failed: %s", err)
		return false
	}
	log.Info("Challenge: %s", cipher)
	c.Write([]byte(cipher))
	log.Info("Challenge sent to client")
	answer := c.ReadSize(tokenLenth)
	if answer != token {
		return false
	}
	return true
}
