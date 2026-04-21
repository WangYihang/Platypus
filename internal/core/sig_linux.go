package core

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"

	"github.com/WangYihang/Platypus/internal/utils/ui"
)

func Signal() {
	// Capture Signal
	c := make(chan os.Signal, 1)
	signal.Notify(
		c,
		syscall.SIGTSTP,
		syscall.SIGTERM,
		os.Interrupt,
		syscall.SIGWINCH,
	)

	go func() {
		for {
			switch sig := <-c; sig {
			case syscall.SIGTSTP:
				if ui.PromptYesNo("syscall.SIGTERM, Exit?") {
					Shutdown()
				}
			case syscall.SIGTERM:
				if ui.PromptYesNo("syscall.SIGTERM, Exit?") {
					Shutdown()
				}
			case os.Interrupt:
				if ui.PromptYesNo("os.Interrupt, Exit?") {
					Shutdown()
				}
			case syscall.SIGWINCH:
				if Ctx.CurrentAgent != nil {
					columns, rows, _ := term.GetSize(0)
					Ctx.CurrentAgent.(*AgentClient).NotifyPlatypusWindowSize(columns, rows)
				}
			}
		}
	}()
}
