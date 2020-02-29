package context

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/WangYihang/Platypus/lib/util/log"
)

type Context struct {
	Servers        map[string](*TCPServer)
	Current        *TCPClient
	CommandPrompt  string
	BlockSameIP    int
	AllowInterrupt bool
}

var Ctx *Context

func Signal() {
	// Capture Signal
	c := make(chan os.Signal, 1)

	// Notify SIGHUP
	signal.Notify(c, os.Interrupt, syscall.SIGTSTP)

	log.Error("Signal installed")
	// signal.Notify(c, syscall.SIGTSTP)

	go func() {
		for {
			switch sig := <-c; sig {
			case os.Interrupt:
				if Ctx.AllowInterrupt {
					// CTRL C
					log.Error("%s signal found", sig)
					i := Ctx.Current.Write([]byte("\x03"))
					log.Error("%d bytes written", i)

				}
			case syscall.SIGTSTP:
				if Ctx.AllowInterrupt {
					// CTRL Z
					log.Error("%s signal found", sig)
					i := Ctx.Current.Write([]byte("\x1A"))
					log.Error("%d bytes written", i)
				}
			}
		}
	}()
}

func CreateContext() {
	if Ctx == nil {
		Ctx = &Context{
			Servers:        make(map[string](*TCPServer)),
			Current:        nil,
			CommandPrompt:  ">> ",
			BlockSameIP:    1,
			AllowInterrupt: false,
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
	for _, server := range Ctx.Servers {
		(*server).DeleteTCPClient(c)
	}
}
