package reverse

type Context struct {
	Servers       map[string](*ReverseServer)
	Current       *ReverseClient
	CommandPrompt string
}

var Ctx *Context

func CreateContext() {
	if Ctx == nil {
		Ctx = &Context{
			Servers:       make(map[string](*ReverseServer)),
			Current:       nil,
			CommandPrompt: ">> ",
		}
	}
}

func GetContext() *Context {
	return Ctx
}

func (ctx Context) AddServer(s *ReverseServer) {
	ctx.Servers[s.Hash()] = s
}

func (ctx Context) DeleteServer(s *ReverseServer) {
	s.Stop()
	delete(ctx.Servers, s.Hash())
}

func (ctx Context) DeleteTCPClient(c *ReverseClient) {
	for _, server := range Ctx.Servers {
		server.DeleteTCPClient(&c.TCPClient)
	}
}
