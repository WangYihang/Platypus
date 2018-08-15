package dispatcher

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/model"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func (ctx Dispatcher) Interact(args []string) {
	if context.Ctx.Current == nil {
		log.Error("Interactive session is not set, please use `Jump` command to set the interactive Interact")
		return
	}
	log.Info("Interacting with %s", context.Ctx.Current.Desc())

	ChannelOpen := true

	go func() {
		// Read commands from client channel, Write to stdout
		for {
			select {
			case data, ok := <-context.Ctx.Current.OutPipe:
				if !ok {
					log.Error("Channel of %s closed", context.Ctx.Current.Desc())
					ChannelOpen = false
					return
				}
				fmt.Print(string(data))
			}
		}
	}()
	// Signal Handler
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGSTOP, syscall.SIGTSTP)

	go func(client *model.Client) {
		select {
		case s := <-c:
			log.Info("Signal %s captured, sending signal via network", s)
			switch s {
			case os.Interrupt:
				client.Conn.Write([]byte("\u0003"))
			case syscall.SIGSTOP:
				client.Conn.Write([]byte("\u001A"))
			case syscall.SIGTSTP:
				client.Conn.Write([]byte("\u001A"))
			}
		}
	}(context.Ctx.Current)

	// Read commands from stdin, Write to client channel
	for {
		if context.Ctx.Current == nil {
			return
		}
		inputReader := bufio.NewReader(os.Stdin)
		command, err := inputReader.ReadString('\n')
		if err != nil {
			log.Error("Read from stdin failed")
			continue
		}
		if command == "exit\n" {
			break
		}
		if command == "shell\n" {
			command = "python -c 'import pty;pty.spawn(\"/bin/sh\")'\n"
		}
		if ChannelOpen {
			context.Ctx.Current.InPipe <- []byte(command)
		} else {
			// Channel closed, do cleanup
			context.Ctx.DeleteClient(context.Ctx.Current)
			return
		}
	}
}

func (ctx Dispatcher) InteractHelp(args []string) {
	fmt.Println("Usage of Interact")
	fmt.Println("\tInteract")
}

func (ctx Dispatcher) InteractDesc(args []string) {
	fmt.Println("Interact")
	fmt.Println("\tThis command will pop up a interactive session, you can communicate with it via stdin/stdout")
}
