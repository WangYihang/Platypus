package dispatcher

import (
	"fmt"
	"os"
	"time"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/ui"
	"github.com/vbauerster/mpb/v5"
	"github.com/vbauerster/mpb/v5/decor"
)

func fileExists(filename string) bool {
    info, err := os.Stat(filename)
    if os.IsNotExist(err) {
        return false
    }
    return !info.IsDir()
}

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

	if fileExists(dst) {
		if !ui.PromptYesNo("The target file exists, do you want to overwrite it?") {
			return
		}
    }

	dstfd, err := os.OpenFile(dst, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Error("Failed to open target file: %s", err)
		return
	}
	defer dstfd.Close()

	log.Info("Downloading %s to %s from client: %s", src, dst, context.Ctx.Current.OnelineDesc())
	totalBytes, err := context.Ctx.Current.FileSize(src)
	if err != nil {
		log.Error("Failed to get file size: %s", err)
		return
	}
	log.Info("Filesize: %d", totalBytes)

	// Progress bar
	p := mpb.New(
		mpb.WithWidth(64),
		mpb.WithRefreshRate(180*time.Millisecond),
	)

	bar := p.AddBar(int64(totalBytes), mpb.BarStyle("[=>-|"),
		mpb.PrependDecorators(
			decor.CountersKibiByte("% .2f / % .2f"),
		),
		mpb.AppendDecorators(
			decor.EwmaETA(decor.ET_STYLE_HHMMSS, 90),
			decor.Name(" ] "),
			decor.EwmaSpeed(decor.UnitKB, "% .2f", 60),
		),
	)

	blockSize := 1024 * 64
	firstBlockSize := totalBytes % blockSize
	n := 0
	
	// Read from remote client
	content, err := context.Ctx.Current.ReadfileEx(src, 0, firstBlockSize)
	if err != nil {
		log.Error("%s", err)
		return
	}
	if n, err = dstfd.Write([]byte(content)); err != nil {
		log.Error("Failed to write data to target file: %s", err)
		return
	}
	bar.IncrBy(n)

	for i := 0; i < totalBytes / blockSize; i++ {
		content, err := context.Ctx.Current.ReadfileEx(src, firstBlockSize + i * blockSize, blockSize)
		if err != nil {
			log.Error("%s", err)
			return
		}
		if n, err = dstfd.Write([]byte(content)); err != nil {
			log.Error("Failed to write data to target file: %s", err)
			return
		}
		bar.IncrBy(n)
	}

	p.Wait()

	// //  Write to local file
	// err = ioutil.WriteFile(dst, []byte(content), 0644)
	// if err != nil {
	// 	log.Error("%s", err)
	// 	return
	// }
	// log.Info("%d bytes is written", len(content))
}

func (dispatcher Dispatcher) DownloadHelp(args []string) {
	fmt.Println("Usage of Download")
	fmt.Println("\tDownload [SRC] [DST]")
}

func (dispatcher Dispatcher) DownloadDesc(args []string) {
	fmt.Println("Download")
	fmt.Println("\tDownload file from remote client (the current client) to local machine")
}
