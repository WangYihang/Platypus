package reverse

import (
	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/str"
)

type ReverseClient struct {
	TCPClient context.Client
}

func (c *ReverseClient) Readfile(filename string) string {
	if c.FileExists(filename) {
		return c.SystemToken("cat " + filename)
	} else {
		log.Error("No such file")
		return ""
	}
}

func (c *ReverseClient) FileExists(path string) bool {
	return c.SystemToken("ls "+path) == path+"\n"
}

func (c *ReverseClient) System(command string) {
	c.TCPClient.Conn.Write([]byte(command + "\n"))
}

func (c *ReverseClient) SystemToken(command string) string {
	tokenA := str.RandomString(0x10)
	tokenB := str.RandomString(0x10)
	input := "echo " + tokenA + " && " + command + "; echo " + tokenB
	c.System(input)
	c.TCPClient.ReadUntil(tokenA)
	output := c.TCPClient.ReadUntil(tokenB)
	log.Info(output)
	return output
}
