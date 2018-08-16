package context

type Context struct {
	Servers       map[string](*BaseTCPServer)
	Current       *Client
	CommandPrompt string
}

var Ctx *Context

func CreateContext() {
	if Ctx == nil {
		Ctx = &Context{
			Servers:       make(map[string](*BaseTCPServer)),
			Current:       nil,
			CommandPrompt: ">> ",
		}
	}
}

func GetContext() *Context {
	return Ctx
}

func (ctx Context) AddServer(s *BaseTCPServer) {
	ctx.Servers[s.Hash] = s
}

func (ctx Context) DeleteServer(s *BaseTCPServer) {
	s.Stop()
	delete(ctx.Servers, s.Hash)
}

func (ctx Context) DeleteClient(c *Client) {
	for _, server := range Ctx.Servers {
		server.DeleteClient(c)
	}
}
