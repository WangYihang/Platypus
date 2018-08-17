package main

import (
	"encoding/gob"
	"net"
	"sync"

	"github.com/WangYihang/Platypus/lib/model/platypus"
	"github.com/WangYihang/Platypus/lib/util/crypto"
	"github.com/WangYihang/Platypus/lib/util/log"
)

var Key []byte

func init() {
	Key = []byte("VnwkyMTUgmzVxUi6")
}

func HandleMessage(conn net.Conn, message platypus.Message) {
	switch message.Type {
	case platypus.AUTH:
		token, err := crypto.Decrypt(Key, message.Content)
		if err != nil {
			log.Error("Decrypt challenge failed, %s", err)
		}
		log.Success("Token: %s", token)
		conn.Write(token)
	}
}

func main() {
	conn, err := net.Dial("tcp", "127.0.0.1:4444")
	if err != nil {
		log.Error("Connection failed, %s", err)
		return
	}
	lock := new(sync.Mutex)
	decoder := gob.NewDecoder(conn)
	for {
		var message platypus.Message
		lock.Lock()
		err = decoder.Decode(&message)
		lock.Unlock()
		if err != nil {
			if err.Error() == "EOF" {
				log.Error("Read from server failed: %s", err)
				break
			}
			log.Error("decode error: %s", err)
			continue
		}
		HandleMessage(conn, message)
	}

	conn.Close()
}
