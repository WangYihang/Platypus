package dispatcher

import (
	"fmt"
	"strconv"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func (dispatcher Dispatcher) BlockSameIP(args []string) {
	if len(args) != 1 {
		log.Error("Arguments error, use `Help BlockSameIP` to get more information")
		dispatcher.BlockSameIPHelp([]string{})
		return
	}
	parseInt,err := strconv.Atoi(args[0])
	if err != nil {
		log.Error("Something error")
		return
	}
	if parseInt == 1 {
		log.Success("BlockSameIP set to 1, will only accept one client from every unique IP")
		context.Ctx.BlockSameIP = 1
	} else if parseInt == 0 {
		log.Success("BlockSameIP set to 0, every IP can have many clients")
		context.Ctx.BlockSameIP = 0
	}
}

func (dispatcher Dispatcher) BlockSameIPHelp(args []string) {
	fmt.Println("Usage of BlockSameIP")
	fmt.Println("\tBlockSameIP [01]")
	fmt.Println("\tWhen BlockSameIP set to 1, will only accept one client from every unique IP, by default")
	fmt.Println("\tWhen BlockSameIP set to 0, every IP can have many clients")
}

func (dispatcher Dispatcher) BlockSameIPDesc(args []string) {
	fmt.Println("BlockSameIP")
	fmt.Println("\tIf a client is online, decline other requests from the same IP")
}
