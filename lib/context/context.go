package context

type Context struct {
	Servers       map[string](*Server)
	Current       *Client
	CommandPrompt string
}

var Ctx *Context

func CreateContext() {
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

func (ctx Context) AddServer(s *Server) {
	ctx.Servers[s.Hash] = s
}

func (ctx Context) DeleteServer(s *Server) {
	s.Stop()
	delete(ctx.Servers, s.Hash)
}

func (ctx Context) DeleteClient(c *Client) {
	for _, server := range Ctx.Servers {
		server.DeleteClient(c)
	}
}
