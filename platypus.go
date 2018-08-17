package main

import (
	"github.com/WangYihang/Platypus/lib/cli/dispatcher"
	"github.com/WangYihang/Platypus/lib/context"
)

func main() {
	// Create context
	context.CreateContext()
	// Run main loop
	dispatcher.Run()
}
