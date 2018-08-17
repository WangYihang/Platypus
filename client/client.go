package main

import (
	"encoding/gob"
	"net"

	"github.com/WangYihang/Platypus/lib/model/platypus"
	"github.com/WangYihang/Platypus/lib/util/crypto"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func main() {
	conn, err := net.Dial("tcp", "127.0.0.1:4444")
	if err != nil {
		log.Error("Connection failed, %s", err)
		return
	}
	key := []byte("VnwkyMTUgmzVxUi6")

	// Decode message
	dec := gob.NewDecoder(conn)
	var message platypus.Message
	err = dec.Decode(&message)
	if err != nil {
		log.Error("decode error: %s", err)
	}

	token, err := crypto.Decrypt(key, message.Content)
	if err != nil {
		log.Error("Decrypt challenge failed, %s", err)
	}
	log.Success("Token: %s", token)
	conn.Write(token)
	conn.Close()
}
