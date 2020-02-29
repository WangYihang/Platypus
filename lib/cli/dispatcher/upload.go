package dispatcher

import (
	"fmt"
)

func (dispatcher Dispatcher) Upload(args []string) {
	// TODO: upload, (use go-pretty to show the upload progress)
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
