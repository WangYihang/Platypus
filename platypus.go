package main

import (
	"time"
	"github.com/WangYihang/Platypus/lib/cli/dispatcher"
	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/config"  
)

func main() {
	// Config loop
	go func(){
		for {
			config.Cfg.Reload()
			time.Sleep(time.Second * 3)
		}
	}()
	// Create context
	context.CreateContext()
	// Run main loop
	dispatcher.Run()
}
