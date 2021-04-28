package context

import (
	"strings"
	"sync"

	"github.com/WangYihang/Platypus/lib/util/config"
	"github.com/WangYihang/Platypus/lib/util/message"
	"github.com/WangYihang/readline"
	"github.com/fatih/color"
	"github.com/gin-gonic/gin"
	"gopkg.in/olahol/melody.v1"
)

type Context struct {
	Servers         map[string](*TCPServer)
	NotifyWebSocket *melody.Melody
	Current         *TCPClient
	CurrentTermite  *TermiteClient
	CommandPrompt   string
	RLInstance      *readline.Instance
	Interacting     *sync.Mutex
	// Set later in platypus.go
	Distributor *Distributor
	RESTful     *gin.Engine
	Config      *config.Config
}

var Ctx *Context

func CreateContext() {
	if Ctx == nil {
		Ctx = &Context{
			Servers:         make(map[string](*TCPServer)),
			NotifyWebSocket: nil,
			Current:         nil,
			CurrentTermite:  nil,
			CommandPrompt:   color.CyanString("» "),
			RLInstance:      nil,
			Interacting:     new(sync.Mutex),
		}
	}
	// Signal Handler
	Signal()
	// Register gob
	message.RegisterGob()
}

func GetContext() *Context {
	return Ctx
}

func (ctx Context) AddServer(s *TCPServer) {
	ctx.Servers[(*s).Hash] = s
}

func (ctx Context) DeleteServer(s *TCPServer) {
	(*s).Stop()
	delete(ctx.Servers, (*s).Hash)
}

func (ctx Context) DeleteTCPClient(c *TCPClient) {
	// recover command prompt
	ctx.RLInstance.SetPrompt(color.CyanString("» "))
	for _, server := range Ctx.Servers {
		(*server).DeleteTCPClient(c)
	}
}

func (ctx Context) DeleteTermiteClient(c *TermiteClient) {
	ctx.RLInstance.SetPrompt(color.CyanString("» "))
	for _, server := range Ctx.Servers {
		(*server).DeleteTermiteClient(c)
	}
}

func (ctx Context) FindTCPClientByAlias(alias string) *TCPClient {
	if alias == "" {
		return nil
	}
	for _, server := range Ctx.Servers {
		for _, client := range (*server).GetAllTCPClients() {
			if strings.HasPrefix(client.Alias, strings.ToLower(alias)) {
				return client
			}
		}
	}
	return nil
}

func (ctx Context) FindTermiteClientByAlias(alias string) *TermiteClient {
	if alias == "" {
		return nil
	}
	for _, server := range Ctx.Servers {
		for _, client := range (*server).GetAllTermiteClients() {
			if strings.HasPrefix(client.Alias, strings.ToLower(alias)) {
				return client
			}
		}
	}
	return nil
}

func (ctx Context) FindTCPClientByHash(hash string) *TCPClient {
	if hash == "" {
		return nil
	}
	for _, server := range Ctx.Servers {
		for _, client := range (*server).GetAllTCPClients() {
			if strings.HasPrefix(client.Hash, strings.ToLower(hash)) {
				return client
			}
		}
	}
	return nil
}

func (ctx Context) FindTermiteClientByHash(hash string) *TermiteClient {
	if hash == "" {
		return nil
	}
	for _, server := range Ctx.Servers {
		for _, client := range (*server).GetAllTermiteClients() {
			if strings.HasPrefix(client.Hash, strings.ToLower(hash)) {
				return client
			}
		}
	}
	return nil
}

func (ctx Context) FindServerByHash(hash string) *TCPServer {
	if hash == "" {
		return nil
	}
	for _, server := range Ctx.Servers {
		if strings.HasPrefix(server.Hash, strings.ToLower(hash)) {
			return server
		}
	}
	return nil
}

func (ctx Context) FindServerListeningAddressByRouteKey(routeKey string) string {
	for k, v := range ctx.Distributor.Route {
		if v == routeKey {
			return k
		}
	}
	return ""
}
