package context

import "github.com/WangYihang/Platypus/lib/model"

var Servers map[string](*model.Server)
var Current *model.Client

var CommandPrompt = ">> "

func init() {
	Servers = make(map[string](*model.Server))
}
