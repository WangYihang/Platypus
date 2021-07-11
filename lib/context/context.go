package context

import (
	"net"
	"os"
	"strings"
	"sync"

	"github.com/WangYihang/Platypus/lib/util/config"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/message"
	"github.com/WangYihang/Platypus/lib/util/str"
	"github.com/WangYihang/Platypus/lib/util/ui"
	"github.com/WangYihang/readline"
	"github.com/fatih/color"
	"github.com/gin-gonic/gin"
	"gopkg.in/olahol/melody.v1"
)

type TunnelConfig struct {
	Termite *TermiteClient
	Address string
	Server  *net.Listener
}

type TunnelInstance struct {
	Termite *TermiteClient
	Conn    *net.Conn
}

type Context struct {
	Servers         map[string](*TCPServer)
	NotifyWebSocket *melody.Melody
	Current         *TCPClient
	CurrentTermite  *TermiteClient
	CommandPrompt   string
	RLInstance      *readline.Instance
	Interacting     *sync.Mutex
	TunnelConfig    map[string]TunnelConfig
	TunnelInstance  map[string]TunnelInstance
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
			TunnelConfig:    make(map[string]TunnelConfig),
			TunnelInstance:  make(map[string]TunnelInstance),
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

func Shutdown() {
	if len(Ctx.Servers) > 0 && !ui.PromptYesNo("There are listening servers, do you really want to exit?") {
		return
	}
	for _, server := range Ctx.Servers {
		(*server).Stop()
		delete(Ctx.Servers, (*server).Hash)
	}
	os.Exit(0)
}

func AddTunnelConfig(termite *TermiteClient, local_address string, remote_address string) {
	tunnel, err := net.Listen("tcp", local_address)
	if err != nil {
		log.Error(err.Error())
		return
	} else {
		Ctx.TunnelConfig[local_address] = TunnelConfig{
			Termite: termite,
			Address: remote_address,
			Server:  &tunnel,
		}
	}

	go func() {
		for {
			conn, _ := tunnel.Accept()

			token := str.RandomString(0x10)

			termite.EncoderLock.Lock()
			err := termite.Encoder.Encode(message.Message{
				Type: message.TUNNEL_CONNECT,
				Body: message.BodyTunnelConnect{
					Token:   token,
					Address: remote_address,
				},
			})
			termite.EncoderLock.Unlock()

			if err == nil {
				Ctx.TunnelInstance[token] = TunnelInstance{
					Conn:    &conn,
					Termite: termite,
				}
			}
		}
	}()
}

func WriteTunnel(termite *TermiteClient, token string, data []byte) {
	termite.AtomLock.Lock()
	defer func() { termite.AtomLock.Unlock() }()

	termite.EncoderLock.Lock()
	err := termite.Encoder.Encode(message.Message{
		Type: message.TUNNEL_DATA,
		Body: message.BodyTunnelData{
			Token: token,
			Data:  data,
		},
	})
	termite.EncoderLock.Unlock()

	if err != nil {
		log.Error("Network error: %s", err)
	}
}

// func DeleteTunnelConfig(local_host string, local_port uint16, remote_host string, remote_port uint16) {
// 	local_address := fmt.Sprintf("%s:%d", local_host, local_port)
// 	remote_address := fmt.Sprintf("%s:%d", remote_host, remote_port)

// 	log.Info("Unmapping from remote %s to local %s", remote_address, local_address)

// 	if tc, exists := Ctx.TunnelConfig[local_address]; exists {
// 		c.AtomLock.Lock()
// 		defer func() { c.AtomLock.Unlock() }()

// 		c.EncoderLock.Lock()
// 		err := c.Encoder.Encode(message.Message{
// 			Type: message.TUNNEL_DELETE,
// 			Body: message.BodyTunnelDelete{
// 				Key:         key,
// 				TermiteHash: c.Hash,
// 			},
// 		})
// 		c.EncoderLock.Unlock()

// 		if err != nil {
// 			log.Error("Network error: %s", err)
// 		} else {
// 			delete(Ctx.TunnelConfig, local_address)
// 		}
// 	} else {
// 		log.Info("No such tunnel from remote %s to local %s", remote_address, local_address)
// 	}
// }
