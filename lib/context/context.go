package context

import (
	"net"

	"github.com/WangYihang/Platypus/lib/util/log"
)

type Context struct {
	Servers       map[string](*Server)
	Current       *Client
	CommandPrompt string
}

var Ctx *Context

func InitContext() {
	if Ctx == nil {
		Ctx = &Context{
			Servers:       make(map[string](*Server)),
			Current:       nil,
			CommandPrompt: ">> ",
		}
	}
}

func GetContext() *Context {
	return Ctx
}

func (ctx Context) DeleteClient(c *Client) {
	for _, server := range Ctx.Servers {
		server.DeleteClient(c)
	}
}

func (ctx Context) RunServer(server *Server, listener *net.TCPListener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		client := CreateClient(conn)
		log.Info("New client %s Connected", client.Desc())
		server.AddClient(client)
	}
}
