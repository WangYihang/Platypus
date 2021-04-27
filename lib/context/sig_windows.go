package context

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/WangYihang/Platypus/lib/util/log"
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
				log.Info("syscall.SIGTERM, Exit?")
			case os.Interrupt:
				log.Info("os.Interrupt, Exit?")
			}
		}
	}()
}
