package context

import (
	"net"
	"os"
	"strings"
	"sync"

	"github.com/WangYihang/Platypus/internal/utils/config"
	"github.com/WangYihang/Platypus/internal/utils/log"
	"github.com/WangYihang/Platypus/internal/utils/message"
	"github.com/WangYihang/Platypus/internal/utils/str"
	"github.com/WangYihang/Platypus/internal/utils/ui"
	"github.com/WangYihang/readline"
	"github.com/armon/go-socks5"
	"github.com/fatih/color"
	"github.com/gin-gonic/gin"
	"gopkg.in/olahol/melody.v1"
)

type PullTunnelConfig struct {
	Termite *TermiteClient
	Address string
	Server  *net.Listener
}

type PullTunnelInstance struct {
	Termite *TermiteClient
	Conn    *net.Conn
}

type PushTunnelConfig struct {
	Termite *TermiteClient
	Address string
}

type PushTunnelInstance struct {
	Termite *TermiteClient
	Conn    *net.Conn
}

type Context struct {
	Servers            map[string](*TCPServer)
	NotifyWebSocket    *melody.Melody
	Current            *TCPClient
	CurrentTermite     *TermiteClient
	CommandPrompt      string
	RLInstance         *readline.Instance
	Interacting        *sync.Mutex
	PullTunnelConfig   map[string]PullTunnelConfig
	PullTunnelInstance map[string]PullTunnelInstance
	PushTunnelConfig   map[string]PushTunnelConfig
	PushTunnelInstance map[string]PushTunnelInstance
	Socks5Servers      map[string](*socks5.Server)
	MessageQueue       map[string](chan message.Message)
	// Set later in platypus.go
	Distributor *Distributor
	RESTful     *gin.Engine
	Config      *config.Config
}

var Ctx *Context

func CreateContext() {
	if Ctx == nil {
		Ctx = &Context{
			Servers:            make(map[string](*TCPServer)),
			NotifyWebSocket:    nil,
			Current:            nil,
			CurrentTermite:     nil,
			CommandPrompt:      color.CyanString("» "),
			RLInstance:         nil,
			Interacting:        new(sync.Mutex),
			PullTunnelConfig:   make(map[string]PullTunnelConfig),
			PullTunnelInstance: make(map[string]PullTunnelInstance),
			PushTunnelConfig:   make(map[string]PushTunnelConfig),
			PushTunnelInstance: make(map[string]PushTunnelInstance),
			Socks5Servers:      make(map[string]*socks5.Server),
			MessageQueue:       make(map[string](chan message.Message)),
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

func AddPushTunnelConfig(termite *TermiteClient, local_address string, remote_address string) {
	log.Info("Mapping local (%s) to remote (%s)", local_address, remote_address)

	termite.LockAtom()
	defer termite.UnlockAtom()

	err := termite.Send(message.Message{
		Type: message.PUSH_TUNNEL_CREATE,
		Body: message.BodyPushTunnelCreate{
			Address: remote_address,
		},
	})

	if err != nil {
		log.Error(err.Error())
	} else {
		Ctx.PushTunnelConfig[remote_address] = PushTunnelConfig{
			Termite: termite,
			Address: local_address,
		}
	}
}

func AddPullTunnelConfig(termite *TermiteClient, local_address string, remote_address string) {
	log.Info("Mapping remote (%s) to local (%s)", remote_address, local_address)
	tunnel, err := net.Listen("tcp", local_address)
	if err != nil {
		log.Error(err.Error())
		return
	} else {
		Ctx.PullTunnelConfig[local_address] = PullTunnelConfig{
			Termite: termite,
			Address: remote_address,
			Server:  &tunnel,
		}
	}

	go func() {
		for {
			conn, _ := tunnel.Accept()

			token := str.RandomString(0x10)

			err := termite.Send(message.Message{
				Type: message.PULL_TUNNEL_CONNECT,
				Body: message.BodyPullTunnelConnect{
					Token:   token,
					Address: remote_address,
				},
			})

			if err == nil {
				Ctx.PullTunnelInstance[token] = PullTunnelInstance{
					Conn:    &conn,
					Termite: termite,
				}
			}
		}
	}()
}

func WriteTunnel(termite *TermiteClient, token string, data []byte) {
	termite.LockAtom()
	defer termite.UnlockAtom()

	err := termite.Send(message.Message{
		Type: message.PULL_TUNNEL_DATA,
		Body: message.BodyPullTunnelData{
			Token: token,
			Data:  data,
		},
	})

	if err != nil {
		log.Error("Network error: %s", err)
	}
}

func StartSocks5Server(local_address string) error {
	// Create tcp listener
	socks5ServerListener, err := net.Listen("tcp", local_address)
	if err != nil {
		return err
	}
	// Create socks5 server
	server, err := socks5.New(&socks5.Config{})
	if err != nil {
		return err
	}
	Ctx.Socks5Servers[local_address] = server
	// Start socks5 server
	go server.Serve(socks5ServerListener)
	log.Success("Socks server started at: %s", local_address)
	return nil
}
