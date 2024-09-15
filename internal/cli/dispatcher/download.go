package dispatcher

import (
	"fmt"
	"os"
	"time"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/utils/log"
	"github.com/WangYihang/Platypus/internal/utils/ui"
	"github.com/vbauerster/mpb/v6"
	"github.com/vbauerster/mpb/v6/decor"
)

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func (dispatcher commandDispatcher) Download(args []string) {
	if len(args) != 2 {
		log.Error("Arguments error, use `Help Download` to get more information")
		dispatcher.DownloadHelp([]string{})
		return
	}

	if context.Ctx.Current == nil && context.Ctx.CurrentTermite == nil {
		log.Error("The current client is not set, please use `Jump` command to select the current client")
		return
	}

	src := args[0]
	dst := args[1]

	if fileExists(dst) {
		if !ui.PromptYesNo("The target file exists, do you want to overwrite it?") {
			return
		}
	}

	dstfd, err := os.OpenFile(dst, os.O_APPEND|os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		log.Error("Failed to open target file: %s", err)
		return
	}
	defer dstfd.Close()

	if context.Ctx.Current != nil {
		log.Info("Downloading %s to %s from client: %s", src, dst, context.Ctx.Current.OnelineDesc())
		totalBytes, err := context.Ctx.Current.FileSize(src)
		if err != nil {
			log.Error("Failed to get file size: %s", err)
			return
		}
		if totalBytes > 1 {
			log.Info("Filesize: %d bytes", totalBytes)
		} else {
			log.Info("Filesize: %d byte", totalBytes)
		}

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

		blockSize := 0x1000 // 4KB
		firstBlockSize := totalBytes % blockSize
		n := 0

		// Read from remote client
		start := time.Now()
		content, err := context.Ctx.Current.ReadFileEx(src, 0, firstBlockSize)
		if err != nil {
			bar.Abort(true)
			log.Error("%s", err)
			return
		}
		if n, err = dstfd.Write([]byte(content)); err != nil {
			bar.Abort(true)
			log.Error("Failed to write data to target file: %s", err)
			return
		}
		bar.IncrBy(n)
		bar.DecoratorEwmaUpdate(time.Since(start))

		for i := 0; i < totalBytes/blockSize; i++ {
			start = time.Now()
			content, err := context.Ctx.Current.ReadFileEx(src, firstBlockSize+i*blockSize, blockSize)
			if err != nil {
				bar.Abort(true)
				log.Error("%s", err)
				return
			}
			if n, err = dstfd.Write([]byte(content)); err != nil {
				bar.Abort(true)
				log.Error("Failed to write data to target file: %s", err)
				return
			}
			bar.IncrBy(n)
			bar.DecoratorEwmaUpdate(time.Since(start))
		}
		p.Wait()
		return
	}

	if context.Ctx.CurrentTermite != nil {
		log.Info("Downloading %s to %s from client: %s", src, dst, context.Ctx.CurrentTermite.OnelineDesc())
		totalBytes, err := context.Ctx.CurrentTermite.FileSize(src)
		if err != nil {
			log.Error("Failed to get file size: %s", err)
			return
		}
		if totalBytes > 1 {
			log.Info("Filesize: %d bytes", totalBytes)
		} else {
			log.Info("Filesize: %d byte", totalBytes)
		}

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
		for i := int64(0); i < totalBytes; i += blockSize {
			start := time.Now()
			content, err := context.Ctx.CurrentTermite.ReadFileEx(src, i, blockSize)
			if err != nil {
				log.Error(err.Error())
				return
			}
			n, err := dstfd.Write([]byte(content))
			if err != nil {
				log.Error("Failed to write data to target file: %s", err)
				return
			}
			bar.IncrBy(n)
			bar.DecoratorEwmaUpdate(time.Since(start))
		}
		p.Wait()
		return
	}

}

func (dispatcher commandDispatcher) DownloadHelp(args []string) {
	fmt.Println("Usage of Download")
	fmt.Println("\tDownload [SRC] [DST]")
}

func (dispatcher commandDispatcher) DownloadDesc(args []string) {
	fmt.Println("Download")
	fmt.Println("\tDownload file from remote client (the current client) to local machine")
}
