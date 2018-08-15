package dispatcher

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/WangYihang/Platypus/lib/context"
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
		command = strings.TrimSpace(command)
		if command == "exit" {
			break
		}
		if command == "shell" {
			command = "python -c 'import pty;pty.spawn(\"/bin/sh\")'"
		}
		if command == "^C" {
			command = "\x03"
		}
		if command == "^Z" {
			command = "\x1A"
		}
		if strings.HasPrefix(command, "^V") {
			command = "\x1B\x1B\x1B" + command[2:] + "\r"
		}
		if ChannelOpen {
			context.Ctx.Current.InPipe <- []byte(command + "\n")
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
	fmt.Println("\tPop up a interactive session, you can communicate with it via stdin/stdout")
}
