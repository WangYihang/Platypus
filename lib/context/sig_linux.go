package context

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/WangYihang/Platypus/lib/util/log"
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
				log.Info("syscall.SIGTERM, Exit?")
			case syscall.SIGTERM:
				log.Info("syscall.SIGTERM, Exit?")
			case os.Interrupt:
				log.Info("os.Interrupt, Exit?")
			case syscall.SIGWINCH:
				if Ctx.CurrentTermite != nil {
					columns, rows, _ := term.GetSize(0)
					Ctx.CurrentTermite.NotifyPlatypusWindowSize(columns, rows)
				}
			}
		}
	}()
}
