package cmd

import (
	"bufio"
	"os"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
	"golang.org/x/term"

	"github.com/spf13/cobra"
)

var interactCmd = &cobra.Command{
	Use:   "Interact",
	Short: "Pop up an interactive session",
	Run: func(cmd *cobra.Command, args []string) {
		if core.Ctx.Current == nil && core.Ctx.CurrentTermite == nil {
			log.Error("Interactive session is not set, please use `Jump` to set it")
			return
		}

		if core.Ctx.Current != nil {
			current := core.Ctx.Current.(*core.TCPClient)
			log.Info("Interacting with %s", current.FullDesc())

			current.GetInteractingLock().Lock()
			defer func() { current.GetInteractingLock().Unlock() }()
			current.SetInteractive(true)
			defer func() { current.SetInteractive(false) }()

			core.Ctx.Interacting.Lock()
			defer func() { core.Ctx.Interacting.Unlock() }()

			if current.GetPtyEstablished() {
				log.Info("Setting attacker terminal to raw mode")
				oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
				if err != nil {
					log.Error("Failed to set terminal to raw mode: %s", err)
					return
				}
				defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

				var cont bool = true

				go func() {
					for current.GetInteractive() && current.GetPtyEstablished() && cont {
						current.GetConn().SetReadDeadline(time.Time{})
						m := make([]byte, 1)
						n, err := current.ReadConnLock(m)
						if err == nil {
							os.Stdout.Write(m[0:n])
						}
					}
				}()

				magic := "platyquit"
				firstLine := true
				inputQueueIndex := 0
				inputQueueLength := len(magic) + 1
				inputQueue := make([]byte, inputQueueLength)
				firstHint := true

				for current.GetInteractive() && current.GetPtyEstablished() && cont {
					var pattern string
					if firstLine {
						pattern = magic
					} else {
						pattern = "\r" + magic
					}
					matched := false

					if firstHint && strings.Contains(string(inputQueue)+string(inputQueue), "exit") {
						log.Info("You can type `%s` to return to Platypus", magic)
						firstHint = false
					}
					if strings.Contains(string(inputQueue)+string(inputQueue), pattern) {
						matched = true
					}

					m := make([]byte, 1)
					n, err := os.Stdin.Read(m)
					if err == nil {
						for i := 0; i < n; i++ {
							if m[i] == 8 {
								inputQueueIndex--
								if inputQueueIndex < 0 {
									inputQueueIndex = inputQueueLength + inputQueueIndex
								}
							} else {
								if m[i] == 13 && matched {
									firstLine = false
									cont = false
									current.SetPtyEstablished(false)
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
				go func() {
					for current.GetInteractive() && !current.GetPtyEstablished() {
						current.GetConn().SetReadDeadline(time.Time{})
						m := make([]byte, 0x100)
						n, err := current.ReadConnLock(m)
						if err == nil {
							os.Stdout.Write(m[0:n])
						}
					}
				}()
				for current.GetInteractive() && !current.GetPtyEstablished() {
					inputReader := bufio.NewReader(os.Stdin)
					command, _ := inputReader.ReadString('\n')
					command = strings.TrimSpace(command)
					if command == "exit" {
						break
					}
					current.Write([]byte(command + "\n"))
				}
			}
			return
		}

		if core.Ctx.CurrentTermite != nil {
			core.Ctx.CurrentTermite.(*core.TermiteClient).StartShell()
		}
	},
}
