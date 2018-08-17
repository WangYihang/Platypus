package main

import (
	"net"

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
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		log.Error("Read from server failed, %s", err)
	}
	log.Success("%d bytes read from server", n)
	token, err := crypto.Decrypt(key, buffer[:n])
	if err != nil {
		log.Error("Decrypt challenge failed, %s", err)
	}
	log.Success("Token: %s", token)
	conn.Write(token)
	conn.Close()
}
