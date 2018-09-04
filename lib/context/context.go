package context

type Context struct {
	Servers       map[string](*AbstractTCPServer)
	Current       *TCPClient
	CommandPrompt string
}

var Ctx *Context

func CreateContext() {
	if Ctx == nil {
		Ctx = &Context{
			Servers:       make(map[string](*AbstractTCPServer)),
			Current:       nil,
			CommandPrompt: ">> ",
		}
	}
}

func GetContext() *Context {
	return Ctx
}

func (ctx Context) AddServer(s *AbstractTCPServer) {
	ctx.Servers[(*s).Hash()] = s
}

func (ctx Context) DeleteServer(s *AbstractTCPServer) {
	(*s).Stop()
	delete(ctx.Servers, (*s).Hash())
}

func (ctx Context) DeleteTCPClient(c *TCPClient) {
	for _, server := range Ctx.Servers {
		(*server).DeleteTCPClient(c)
	}
}
