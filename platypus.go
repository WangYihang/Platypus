package main

import (
	"github.com/WangYihang/Platypus/lib/cli/dispatcher"
	"github.com/WangYihang/Platypus/lib/model"
)

func main() {
	model.InitContext()
	dispatcher.Run()
}
