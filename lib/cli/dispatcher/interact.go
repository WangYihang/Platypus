package dispatcher

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
	"golang.org/x/term"
)

func (dispatcher Dispatcher) Interact(args []string) {
	if context.Ctx.Current == nil {
		log.Error("Interactive session is not set, please use `Jump` command to set the interactive Interact")
		return
	}
	log.Info("Interacting with %s", context.Ctx.Current.FullDesc())

	// Set to interactive
	context.Ctx.Current.Interacting.Lock()
	defer func() { context.Ctx.Current.Interacting.Unlock() }()
	context.Ctx.Current.Interactive = true
	defer func() { context.Ctx.Current.Interactive = false }()

	context.Ctx.Interacting.Lock()
	defer func() { context.Ctx.Interacting.Unlock() }()

	if context.Ctx.Current.PtyEstablished {
		// Step 4: Enable Raw mode of attacker pty
		log.Info("Setting attacker terminal to raw mode")
		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			panic(err)
		}
		// Restore tty properties
		defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

		var cont bool = true

		// Client output -> Platypus stdout
		go func() {
			for context.Ctx.Current.Interactive && context.Ctx.Current.PtyEstablished && cont {
				context.Ctx.Current.GetConn().SetReadDeadline(time.Time{})
				m := make([]byte, 1)
				n, err := context.Ctx.Current.ReadConnLock(m)
				if err == nil {
					os.Stdout.Write(m[0:n])
				}
			}
		}()
		magic := "exit"
		inputQueueIndex := 0
		inputQueueLength := len(magic) + 2 // + 2 for clrf
		inputQueue := make([]byte, inputQueueLength)
		// Client input <- Platypus stdin
		for context.Ctx.Current.Interactive && context.Ctx.Current.PtyEstablished && cont {
			// Magic exit mantra: 'exit' is typed
			// Check whether user want to exit pty mode
			// BUG: Only works in shell prompt,
			// 		failed in foreground process trying to read from stdin (eg: vim / htop)
			//		failed in nested shell (eg: bash -> ... -> bash)
			if strings.Contains(string(inputQueue)+string(inputQueue), "\n"+magic+"\n") ||
				strings.Contains(string(inputQueue)+string(inputQueue), "\r"+magic+"\r") {
				// Exit Pty
				cont = false
				context.Ctx.Current.PtyEstablished = false
				term.Restore(int(os.Stdin.Fd()), oldState)
				break
			}

			m := make([]byte, 1)
			n, err := os.Stdin.Read(m)
			if err == nil {
				for i := 0; i < n; i++ {
					inputQueue[inputQueueIndex] = m[i]
				}
				inputQueueIndex += n
				inputQueueIndex %= inputQueueLength
				context.Ctx.Current.Write(m[0:n])
			}
		}
	} else {
		log.Error("PTY is not established, drop into normal reverse shell mode")
		// Client output -> Platypus stdout
		go func() {
			for context.Ctx.Current.Interactive && !context.Ctx.Current.PtyEstablished {
				context.Ctx.Current.GetConn().SetReadDeadline(time.Time{})
				m := make([]byte, 0x100)
				n, err := context.Ctx.Current.ReadConnLock(m)
				if err == nil {
					os.Stdout.Write(m[0:n])
				}
			}
		}()

		// Client input <- Platypus stdin
		for context.Ctx.Current.Interactive && !context.Ctx.Current.PtyEstablished {
			inputReader := bufio.NewReader(os.Stdin)
			command, _ := inputReader.ReadString('\n')
			command = strings.TrimSpace(command)
			if command == "exit" {
				break
			}
			context.Ctx.Current.Write([]byte(command + "\n"))
		}
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
