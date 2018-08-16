package dispatcher

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/lib/model"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/timeout"
)

func (dispatcher Dispatcher) Interact(args []string) {
	if model.Ctx.Current == nil {
		log.Error("Interactive session is not set, please use `Jump` command to set the interactive Interact")
		return
	}
	log.Info("Interacting with %s", model.Ctx.Current.Desc())

	// Set to interactive
	model.Ctx.Current.Interactive = true
	inputChannel := make(chan []byte, 1024)

	// write to socket fd
	go func() {
		for {
			select {
			case data := <-inputChannel:
				if model.Ctx.Current == nil || !model.Ctx.Current.Interactive {
					return
				}
				model.Ctx.Current.Write(data)
			}
		}
	}()

	var sleep_time = timeout.GenerateTimeout()

	// read from socket fd
	go func() {
		for {
			if model.Ctx.Current == nil || !model.Ctx.Current.Interactive {
				return
			}

			buffer, is_timeout := model.Ctx.Current.Read(timeout.GenerateTimeout())
			fmt.Print(buffer)

			// Sleep time trade off
			if is_timeout {
				sleep_time = sleep_time * 2
			}
			if sleep_time > time.Microsecond*0x400 {
				sleep_time = timeout.GenerateTimeout()
			}
			time.Sleep(sleep_time)
		}
	}()

	for {
		if model.Ctx.Current == nil || !model.Ctx.Current.Interactive {
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
			model.Ctx.Current.Interactive = false
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

		sleep_time = timeout.GenerateTimeout()
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
