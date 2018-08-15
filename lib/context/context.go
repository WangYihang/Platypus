package context

import (
	"net"

	"github.com/WangYihang/Platypus/lib/model"
	"github.com/WangYihang/Platypus/lib/util/log"
)

type Context struct {
	Servers       map[string](*model.Server)
	Current       *model.Client
	CommandPrompt string
}

var Ctx *Context

func init() {
	if Ctx == nil {
		Ctx = &Context{
			Servers:       make(map[string](*model.Server)),
			Current:       nil,
			CommandPrompt: ">> ",
		}
	}
}

func (ctx Context) DeleteClient(c *model.Client) {
	for _, server := range ctx.Servers {
		server.DeleteClient(c)
	}
	if c == ctx.Current {
		ctx.Current = nil
	}
}

func (ctx Context) RunServer(server *model.Server, listener *net.TCPListener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		client := model.CreateClient(conn)
		log.Info("New client %s Connected", client.Desc())
		server.AddClient(client)
		go Read(client, ctx)
		go Write(client, ctx)
	}
}

func Read(c *model.Client, ctx Context) {
	for {
		buffer := make([]byte, 1024)
		_, err := c.Conn.Read(buffer)
		if err != nil {
			log.Error("Read failed from %s , error message: %s", c.Desc(), err)
			close(c.OutPipe)
			ctx.DeleteClient(c)
			return
		}
		c.OutPipe <- buffer
	}
}

func Write(c *model.Client, ctx Context) {
	for {
		select {
		case data, ok := <-c.InPipe:
			if !ok {
				log.Error("Channel of %s closed", c.Desc())
				close(c.InPipe)
				ctx.DeleteClient(c)
				return
			}
			n, err := c.Conn.Write(data)
			if err != nil {
				log.Error("Write failed to %s , error message: %s", c.Desc(), err)
				close(c.InPipe)
				ctx.DeleteClient(c)
				return
			}
			log.Info("%d bytes sent", n)
		}
	}
}
