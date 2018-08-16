package main

import (
	"github.com/WangYihang/Platypus/lib/cli/dispatcher"
	"github.com/WangYihang/Platypus/lib/context"
)

func main() {
	context.CreateContext()
	dispatcher.Run()
}
