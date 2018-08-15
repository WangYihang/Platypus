package session

import (
	"fmt"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func Read(c Client) {
	for {
		buffer := make([]byte, 1024)
		n, err := c.Conn.Read(buffer)
		if err != nil {
			log.Error("Read failed from %s , error message: %s", context.Current.Desc(), err)
			// Delete this node from online list
			for _, server := range context.Servers {
				for _, client := range server.Clients {
					if client.Hash == c.Hash {
						server.DeleteClient(client)
					}
				}
			}
			// If current refers to this node, set current to nil
			if client.Hash == context.Current.Hash {
				Current = nil
			}
			log.Error("Cleanup finished")
			return
		}
		fmt.Print(buffer)
	}
}

func Write(c Client) {
	for {
		select {
		case data := <-c.Pipe:
			c.Conn.Write(data)
		}
	}
}
