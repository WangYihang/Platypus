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

	current := context.Ctx.Current
	log.Info("Interacting with %s", current.FullDesc())

	// Set to interactive
	current.Interacting.Lock()
	defer func() { current.Interacting.Unlock() }()
	current.Interactive = true
	defer func() { current.Interactive = false }()

	context.Ctx.Interacting.Lock()
	defer func() { context.Ctx.Interacting.Unlock() }()

	if current.PtyEstablished {
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
			for current.Interactive && current.PtyEstablished && cont {
				current.GetConn().SetReadDeadline(time.Time{})
				m := make([]byte, 1)
				n, err := current.ReadConnLock(m)
				if err == nil {
					os.Stdout.Write(m[0:n])
				}
			}
		}()
		magic := "exit"
		firstLine := true
		inputQueueIndex := 0
		inputQueueLength := len(magic) + 1 // + 1 for clrf
		inputQueue := make([]byte, inputQueueLength)
		// Client input <- Platypus stdin
		for current.Interactive && current.PtyEstablished && cont {
			// Magic exit mantra: 'exit' is typed
			// Check whether user want to exit pty mode
			// BUG: Only works in shell prompt,
			// 		failed in foreground process trying to read from stdin (eg: vim / htop)
			//		failed in nested shell (eg: bash -> ... -> bash)
			fmt.Println(inputQueue)
			var pattern string
			if firstLine {
				pattern = magic
			} else {
				pattern = "\r" + magic
			}
			matched := false
			if strings.Contains(string(inputQueue)+string(inputQueue), pattern) {
				// Exit Pty
				matched = true
			}

			m := make([]byte, 1)
			n, err := os.Stdin.Read(m)
			if err == nil {
				for i := 0; i < n; i++ {
					// Backspace
					if m[i] == 8 {
						// inputQueueIndex = int(math.Max(float64(0), float64(inputQueueIndex-1)))
						inputQueueIndex--
						if inputQueueIndex < 0 {
							inputQueueIndex = inputQueueLength + inputQueueIndex
						}
					} else {
						// user typed: `\rexit` + `\r`
						if m[i] == 13 && matched {
							firstLine = false
							cont = false
							current.PtyEstablished = false
							term.Restore(int(os.Stdin.Fd()), oldState)
							break
						}
						inputQueue[inputQueueIndex] = m[i]
						inputQueueIndex += n
						inputQueueIndex %= inputQueueLength
					}
				}
				current.Write(m[0:n])
			}
		}
	} else {
		log.Error("PTY is not established, drop into normal reverse shell mode. You can use `PTY` command to enable PTY mode.")
		// Client output -> Platypus stdout
		go func() {
			for current.Interactive && !current.PtyEstablished {
				current.GetConn().SetReadDeadline(time.Time{})
				m := make([]byte, 0x100)
				n, err := current.ReadConnLock(m)
				if err == nil {
					os.Stdout.Write(m[0:n])
				}
			}
		}()

		// Client input <- Platypus stdin
		for current.Interactive && !current.PtyEstablished {
			inputReader := bufio.NewReader(os.Stdin)
			command, _ := inputReader.ReadString('\n')
			command = strings.TrimSpace(command)
			if command == "exit" {
				break
			}
			current.Write([]byte(command + "\n"))
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
