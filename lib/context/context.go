package context

import (
	"github.com/WangYihang/Platypus/lib/model"
)

type Context struct {
	Servers       map[string](*model.Server)
	Current       *model.Client
	CommandPrompt string
}

var Ctx *Context

func init() {
	Ctx = &Context{
		Servers:       make(map[string](*model.Server)),
		Current:       nil,
		CommandPrompt: ">> ",
	}
}

func (ctx Context) DeleteClient(c *model.Client) {
	for _, server := range ctx.Servers {
		server.DeleteClient(c)
	}
}
