package dispatcher

import (
	"fmt"
)

func (dispatcher Dispatcher) Upload(args []string) {
	fmt.Println("TO BE IMPLEMENTED.")
}

func (dispatcher Dispatcher) UploadHelp(args []string) {
	fmt.Println("Usage of Upload")
	fmt.Println("\tUpload [SRC] [DST]")
}

func (dispatcher Dispatcher) UploadDesc(args []string) {
	fmt.Println("Upload")
	fmt.Println("\tUpload file from local machine to remote server")
}
