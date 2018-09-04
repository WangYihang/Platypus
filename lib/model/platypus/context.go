package platypus

type Context struct {
	Servers       map[string](*PlatypusServer)
	Current       *PlatypusClient
	CommandPrompt string
}

var Ctx *Context

func CreateContext() {
	if Ctx == nil {
		Ctx = &Context{
			Servers:       make(map[string](*PlatypusServer)),
			Current:       nil,
			CommandPrompt: ">> ",
		}
	}
}

func GetContext() *Context {
	return Ctx
}

func (ctx Context) AddServer(s *PlatypusServer) {
	ctx.Servers[s.Hash()] = s
}

func (ctx Context) DeleteServer(s *PlatypusServer) {
	s.Stop()
	delete(ctx.Servers, s.Hash())
}

func (ctx Context) DeleteClient(c *PlatypusClient) {
	for _, server := range Ctx.Servers {
		server.DeleteTCPClient(&c.TCPClient)
	}
}
