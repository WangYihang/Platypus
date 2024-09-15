package dispatcher

import (
	"fmt"
	"os"
	"time"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/utils/log"
	oss "github.com/WangYihang/Platypus/internal/utils/os"
	"github.com/vbauerster/mpb/v6"
	"github.com/vbauerster/mpb/v6/decor"
)

func (dispatcher commandDispatcher) Upload(args []string) {
	if len(args) != 2 {
		log.Error("Arguments error, use `Help Upload` to get more information")
		dispatcher.UploadHelp([]string{})
		return
	}

	if context.Ctx.Current == nil && context.Ctx.CurrentTermite == nil {
		log.Error("The current client is not set, please use `Jump` command to select the current client")
		return
	}

	src := args[0]
	dst := args[1]

	if context.Ctx.Current != nil {

		if context.Ctx.Current.OS == oss.Windows {
			log.Error("Upload command does not support Windows platform")
			return
		}

		context.Ctx.Current.Upload(src, dst, false)

		// TODO: Check file md5 to verify
		log.Success("File %s uploaded to %s", src, dst)
		return
	}

	if context.Ctx.CurrentTermite != nil {
		log.Info("Uploading %s to %s from client: %s", src, dst, context.Ctx.CurrentTermite.OnelineDesc())

		srcfd, err := os.OpenFile(src, os.O_RDONLY, 0644)
		if err != nil {
			log.Error(err.Error())
			return
		}
		fi, _ := srcfd.Stat()
		totalBytes := fi.Size()

		// Progress bar
		p := mpb.New(
			mpb.WithWidth(64),
		)

		bar := p.Add(int64(totalBytes), mpb.NewBarFiller("[=>-|"),
			mpb.PrependDecorators(
				decor.CountersKibiByte("% .2f / % .2f"),
			),
			mpb.AppendDecorators(
				decor.EwmaETA(decor.ET_STYLE_HHMMSS, 60),
				decor.Name(" ] "),
				decor.EwmaSpeed(decor.UnitKB, "% .2f", 60),
			),
		)

		blockSize := int64(0x400 * 512) // 128KB
		buffer := make([]byte, blockSize)

		for i := int64(0); i < totalBytes; i += blockSize {
			start := time.Now()
			n, err := srcfd.Read(buffer)
			if err != nil {
				bar.Abort(true)
				log.Error("%s", err)
				return
			}
			if n, err = context.Ctx.CurrentTermite.WriteFileEx(dst, buffer[0:n]); err != nil {
				log.Error("Failed to write data to target file: %s", err)
				bar.Abort(true)
				return
			}
			bar.IncrBy(n)
			bar.DecoratorEwmaUpdate(time.Since(start))
		}
		p.Wait()
		return
	}
}

func (dispatcher commandDispatcher) UploadHelp(args []string) {
	fmt.Println("Usage of Upload")
	fmt.Println("\tUpload [SRC] [DST]")
}

func (dispatcher commandDispatcher) UploadDesc(args []string) {
	fmt.Println("Upload")
	fmt.Println("\tUpload file from local machine to remote server")
}
