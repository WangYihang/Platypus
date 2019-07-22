package dispatcher

import (
	"fmt"
)

func (dispatcher Dispatcher) Download(args []string) {
	fmt.Println("TO BE IMPLEMENTED.")
}

func (dispatcher Dispatcher) DownloadHelp(args []string) {
	fmt.Println("Usage of Download")
	fmt.Println("\tDownload [SRC] [DST]")
}

func (dispatcher Dispatcher) DownloadDesc(args []string) {
	fmt.Println("Download")
	fmt.Println("\tDownload file from remote server to local machine")
}
