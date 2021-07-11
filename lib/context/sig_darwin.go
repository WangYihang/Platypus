package context

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/WangYihang/Platypus/lib/util/ui"
	"golang.org/x/term"
)

func Signal() {
	// Capture Signal
	c := make(chan os.Signal)
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
				if Ctx.CurrentTermite != nil {
					columns, rows, _ := term.GetSize(0)
					Ctx.CurrentTermite.NotifyPlatypusWindowSize(columns, rows)
				}
			}
		}
	}()
}
