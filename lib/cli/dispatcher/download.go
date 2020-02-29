package dispatcher

import (
	"fmt"
	"strings"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func (dispatcher Dispatcher) Download(args []string) {
	if len(args) != 3 {
		log.Error("Arguments error, use `Help Download` to get more information")
		dispatcher.DownloadHelp([]string{})
		return
	}

	for _, server := range context.Ctx.Servers {
		if strings.HasPrefix((*server).Hash(), strings.ToLower(args[0])) {
			fmt.Println("[SERVER]: \n\t", (*server).FullDesc())
			return
		}
		for _, client := range (*server).GetAllTCPClients() {
			if strings.HasPrefix(client.Hash, strings.ToLower(args[0])) {
				// Check existence of the src file on target machine

				fmt.Println("[CLIENT]: \n\t", client.Desc())

				return
			}
		}
	}
	log.Error("No such node")
}

func (dispatcher Dispatcher) DownloadHelp(args []string) {
	fmt.Println("Usage of Download")
	fmt.Println("\tDownload [SRC] [DST]")
}

func (dispatcher Dispatcher) DownloadDesc(args []string) {
	fmt.Println("Download")
	fmt.Println("\tDownload file from remote server to local machine")
}
