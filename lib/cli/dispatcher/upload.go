package dispatcher

import (
	"fmt"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
	oss "github.com/WangYihang/Platypus/lib/util/os"
)

func (dispatcher Dispatcher) Upload(args []string) {
	if len(args) != 2 {
		log.Error("Arguments error, use `Help Upload` to get more information")
		dispatcher.DownloadHelp([]string{})
		return
	}

	if context.Ctx.Current == nil && context.Ctx.CurrentTermite == nil {
		log.Error("The current client is not set, please use `Jump` command to select the current client")
		return
	}

	if context.Ctx.Current != nil {

		if context.Ctx.Current.OS == oss.Windows {
			log.Error("Upload command does not support Windows platform")
			return
		}

		src := args[0]
		dst := args[1]

		context.Ctx.Current.Upload(src, dst, false)

		// TODO: Check file md5 to verify
		log.Success("File %s uploaded to %s", src, dst)
		return
	}

	if context.Ctx.CurrentTermite != nil {
		log.Error("Download function is to be implemented")
		return
	}
}

func (dispatcher Dispatcher) UploadHelp(args []string) {
	fmt.Println("Usage of Upload")
	fmt.Println("\tUpload [SRC] [DST]")
}

func (dispatcher Dispatcher) UploadDesc(args []string) {
	fmt.Println("Upload")
	fmt.Println("\tUpload file from local machine to remote server")
}
