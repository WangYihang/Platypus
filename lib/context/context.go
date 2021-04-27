package context

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/WangYihang/Platypus/lib/util/config"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/message"
	"github.com/WangYihang/readline"
	"github.com/fatih/color"
	"github.com/gin-gonic/gin"
	"golang.org/x/term"
	"gopkg.in/olahol/melody.v1"
)

type Distributor struct {
	Host       string            `json:"host"`
	Port       uint16            `json:"port"`
	Interfaces []string          `json:"interfaces"`
	Route      map[string]string `json:"route"`
}

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

func Signal() {
	// Capture Signal
	c := make(chan os.Signal)
	signal.Notify(
		c,
		// syscall.SIGTSTP,
		syscall.SIGTERM,
		os.Interrupt,
		syscall.SIGWINCH,
	)

	go func() {
		for {
			switch sig := <-c; sig {
			// case syscall.SIGTSTP:
			// 	log.Info("syscall.SIGTERM, Exit?")
			case syscall.SIGTERM:
				log.Info("syscall.SIGTERM, Exit?")
			case os.Interrupt:
				// if Ctx.Current.PtyEstablished && Ctx.Current.Interactive {
				// 	// Exit pty gracefully
				// }
				log.Info("os.Interrupt, Exit?")
			case syscall.SIGWINCH:
				if Ctx.CurrentTermite != nil {
					columns, rows, _ := term.GetSize(0)
					Ctx.CurrentTermite.NotifyPlatypusWindowSize(columns, rows)
				}
			}
		}
	}()
}

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

func (ctx Context) FindServerListeningAddressByDispatchKey(routeKey string) string {
	for _, server := range Ctx.Servers {
		for _, host := range server.Interfaces {
			if host == routeKey {
				return fmt.Sprintf("%s:%d", host, server.Port)
			}
		}
	}
	return ""
}
