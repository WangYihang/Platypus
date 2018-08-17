package reverse

type Context struct {
	Servers       map[string](*ReverseTCPServer)
	Current       *ReverseClient
	CommandPrompt string
}

var Ctx *Context

func CreateContext() {
	if Ctx == nil {
		Ctx = &Context{
			Servers:       make(map[string](*ReverseTCPServer)),
			Current:       nil,
			CommandPrompt: ">> ",
		}
	}
}

func GetContext() *Context {
	return Ctx
}

func (ctx Context) AddServer(s *ReverseTCPServer) {
	ctx.Servers[s.Hash] = s
}

func (ctx Context) DeleteServer(s *ReverseTCPServer) {
	s.Stop()
	delete(ctx.Servers, s.Hash)
}

func (ctx Context) DeleteClient(c *ReverseClient) {
	for _, server := range Ctx.Servers {
		server.DeleteClient(&c.Client)
	}
}
