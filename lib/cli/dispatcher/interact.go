package dispatcher

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/timeout"
)

func (dispatcher Dispatcher) Interact(args []string) {
	if context.Ctx.Current == nil {
		log.Error("Interactive session is not set, please use `Jump` command to set the interactive Interact")
		return
	}
	log.Info("Interacting with %s", context.Ctx.Current.Desc())

	// Set to interactive
	context.Ctx.Current.Interactive = true
	inputChannel := make(chan []byte, 1024)

	// write to socket fd
	go func() {
		for {
			select {
			case data := <-inputChannel:
				if context.Ctx.Current == nil || !context.Ctx.Current.Interactive {
					return
				}
				context.Ctx.Current.Write(data)
			}
		}
	}()

	var sleepTime = timeout.GenerateTimeout()

	// read from socket fd
	go func() {
		for {
			if context.Ctx.Current == nil || !context.Ctx.Current.Interactive {
				return
			}

			buffer, isTimeout := context.Ctx.Current.Read(timeout.GenerateTimeout())
			fmt.Print(buffer)

			// Sleep time trade off
			if isTimeout {
				sleepTime = sleepTime * 2
			}
			if sleepTime > time.Microsecond*0x400 {
				sleepTime = timeout.GenerateTimeout()
			}
			time.Sleep(sleepTime)
		}
	}()

	for {
		if context.Ctx.Current == nil || !context.Ctx.Current.Interactive {
			return
		}
		// Read command
		inputReader := bufio.NewReader(os.Stdin)
		command, err := inputReader.ReadString('\n')
		if err != nil {
			log.Error("Read from stdin failed")
			continue
		}
		command = strings.TrimSpace(command)
		if command == "exit" {
			context.Ctx.Current.Interactive = false
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
			command = "\x1B" + command[2:] + "\r"
		}
		// Send command
		inputChannel <- []byte(command + "\n")

		sleepTime = timeout.GenerateTimeout()
	}
}

func (dispatcher Dispatcher) InteractHelp(args []string) {
	fmt.Println("Usage of Interact")
	fmt.Println("\tInteract")
}

func (dispatcher Dispatcher) InteractDesc(args []string) {
	fmt.Println("Interact")
	fmt.Println("\tPop up a interactive session, you can communicate with it via stdin/stdout")
}
