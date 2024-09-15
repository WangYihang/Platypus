package context

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/WangYihang/Platypus/internal/utils/ui"
)

func Signal() {
	// Capture Signal
	c := make(chan os.Signal)
	signal.Notify(
		c,
		syscall.SIGTERM,
		os.Interrupt,
	)

	go func() {
		for {
			switch sig := <-c; sig {
			case syscall.SIGTERM:
				if ui.PromptYesNo("os.Interrupt, Exit?") {
					Shutdown()
				}
			case os.Interrupt:
				if ui.PromptYesNo("os.Interrupt, Exit?") {
					Shutdown()
				}
			}
		}
	}()
}
