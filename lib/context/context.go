package context

import (
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/readline"
	"github.com/fatih/color"
)

type Context struct {
	Servers       map[string](*TCPServer)
	Current       *TCPClient
	CommandPrompt string
	RLInstance    *readline.Instance
	Interacting   *sync.Mutex
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
			}
		}
	}()
}

func CreateContext() {
	if Ctx == nil {
		Ctx = &Context{
			Servers:       make(map[string](*TCPServer)),
			Current:       nil,
			CommandPrompt: color.CyanString("» "),
			RLInstance:    nil,
			Interacting:   new(sync.Mutex),
		}
	}
	// Signal Handler
	Signal()
}

func GetContext() *Context {
	return Ctx
}

func (ctx Context) AddServer(s *TCPServer) {
	ctx.Servers[(*s).Hash()] = s
}

func (ctx Context) DeleteServer(s *TCPServer) {
	(*s).Stop()
	delete(ctx.Servers, (*s).Hash())
}

func (ctx Context) DeleteTCPClient(c *TCPClient) {
	// recover command prompt
	ctx.RLInstance.SetPrompt(color.CyanString("» "))
	for _, server := range Ctx.Servers {
		(*server).DeleteTCPClient(c)
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
