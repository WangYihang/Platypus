package platypus

import (
	"encoding/gob"
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

const (
	AUTH = iota
)

type Message struct {
	Type    int
	Content []byte
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
	key := []byte("VnwkyMTUgmzVxUi6")
	tokenLenth := 0x100
	token := str.RandomString(tokenLenth)
	cipher, err := crypto.Encrypt(key, []byte(token))
	if err != nil {
		log.Info("Encrypting token failed: %s", err)
		return false
	}
	message := Message{
		Type:    AUTH,
		Content: cipher,
	}
	c.WriteMessage(message)
	answer := c.ReadSize(tokenLenth)
	if answer != token {
		return false
	}
	return true
}

func (c *PlatypusClient) WriteMessage(i interface{}) {
	encoder := gob.NewEncoder(c.Conn)
	err := encoder.Encode(i)
	if err != nil {
		log.Error("encoderode error: %s", err)
		return
	}
}
