package context

import (
	"os"
	"os/signal"

	// "syscall"

	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/readline"
	"github.com/fatih/color"
)

type Context struct {
	Servers        map[string](*TCPServer)
	Current        *TCPClient
	CommandPrompt  string
	RLInstance     *readline.Instance
	AllowInterrupt bool
}

var Ctx *Context

func Signal() {
	// Capture Signal
	c := make(chan os.Signal, 1)

	// Notify SIGHUP
	signal.Notify(
		c,
		os.Interrupt,
		// syscall.SIGTSTP,
	)

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
				// commented for windows platform
				// windows platform does not support SIGTSTP
				// so the compilation will fail.
				/*
					case syscall.SIGTSTP:
						if Ctx.AllowInterrupt {
							// CTRL Z
							log.Error("%s signal found", sig)
							i := Ctx.Current.Write([]byte("\x1A"))
							log.Error("%d bytes written", i)
						}
				*/
			}
		}
	}()
}

func CreateContext() {
	if Ctx == nil {
		Ctx = &Context{
			Servers:        make(map[string](*TCPServer)),
			Current:        nil,
			CommandPrompt:  color.CyanString("» "),
			RLInstance:     nil,
			AllowInterrupt: true,
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
