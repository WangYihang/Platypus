package dispatcher

import (
	"fmt"
	"io/ioutil"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func (dispatcher Dispatcher) Download(args []string) {
	if len(args) != 2 {
		log.Error("Arguments error, use `Help Download` to get more information")
		dispatcher.DownloadHelp([]string{})
		return
	}

	if context.Ctx.Current == nil {
		log.Error("The current client is not set, please use `Jump` command to select the current client")
		return
	}

	src := args[0]
	dst := args[1]

	log.Info("Downloading %s to %s from client: %s", src, dst, context.Ctx.Current.OnelineDesc())
	// Read from remote client
	content, err := context.Ctx.Current.Readfile(src)
	if err != nil {
		log.Error("%s", err)
	} else {
		//  Write to local file
		err := ioutil.WriteFile(dst, []byte(content), 0644)
		if err != nil {
			log.Error("%s", err)
		}
		log.Info("%d bytes is written", len(content))
	}
}

func (dispatcher Dispatcher) DownloadHelp(args []string) {
	fmt.Println("Usage of Download")
	fmt.Println("\tDownload [SRC] [DST]")
}

func (dispatcher Dispatcher) DownloadDesc(args []string) {
	fmt.Println("Download")
	fmt.Println("\tDownload file from remote client (the current client) to local machine")
}
