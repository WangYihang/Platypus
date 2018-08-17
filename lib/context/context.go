package context

type Context struct {
	Servers       map[string](*TCPServer)
	Current       *Client
	CommandPrompt string
}

var Ctx *Context

func CreateContext() {
	if Ctx == nil {
		Ctx = &Context{
			Servers:       make(map[string](*TCPServer)),
			Current:       nil,
			CommandPrompt: ">> ",
		}
	}
}

func GetContext() *Context {
	return Ctx
}

func (ctx Context) AddServer(s *TCPServer) {
	ctx.Servers[s.Hash] = s
}

func (ctx Context) DeleteServer(s *TCPServer) {
	s.Stop()
	delete(ctx.Servers, s.Hash)
}

func (ctx Context) DeleteClient(c *Client) {
	for _, server := range Ctx.Servers {
		server.DeleteClient(c)
	}
}
