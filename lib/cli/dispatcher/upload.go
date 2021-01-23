package dispatcher

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/vbauerster/mpb/v6"
	"github.com/vbauerster/mpb/v6/decor"
)

func (dispatcher Dispatcher) Upload(args []string) {
	if len(args) != 2 {
		log.Error("Arguments error, use `Help Upload` to get more information")
		dispatcher.DownloadHelp([]string{})
		return
	}

	if context.Ctx.Current == nil {
		log.Error("The current client is not set, please use `Jump` command to select the current client")
		return
	}

	if context.Ctx.Current.OS == context.Windows {
		log.Error("Upload command does not support Windows platform")
		return
	}

	src := args[0]
	dst := args[1]

	// Check existance of remote path
	dstExists, err := context.Ctx.Current.FileExists(dst)
	if err != nil {
		log.Error(err.Error())
		return
	}

	// Check writablity of remote path
	// TODO

	if dstExists {
		log.Error("The target path is occupied, please select another destination")
		return
	}

	// Read local file content
	content, err := ioutil.ReadFile(src)
	if err != nil {
		log.Error(err.Error())
		return
	}

	segmentSize := 0x400

	totalBytes := len(content)
	segments := totalBytes / segmentSize
	overflowedBytes := totalBytes - segments*segmentSize

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

	// Firstly, use redirect `>` to create file, and write the overflowed bytes
	start := time.Now()
	context.Ctx.Current.SystemToken(fmt.Sprintf(
		"echo %s| base64 -d > %s",
		base64.StdEncoding.EncodeToString(content[0:overflowedBytes]),
		dst,
	))
	bar.IncrBy(overflowedBytes)
	bar.DecoratorEwmaUpdate(time.Since(start))

	// Secondly, use `>>` to append all segments left except the final one
	for i := 0; i < segments; i++ {
		start = time.Now()
		context.Ctx.Current.SystemToken(fmt.Sprintf(
			"echo %s| base64 -d >> %s",
			base64.StdEncoding.EncodeToString(content[overflowedBytes+i*segmentSize:overflowedBytes+(i+1)*segmentSize]),
			dst,
		))
		bar.IncrBy(segmentSize)
		bar.DecoratorEwmaUpdate(time.Since(start))
	}
	p.Wait()

	// TODO
	// Check file md5 to verify
	log.Success("File %s uploaded to %s", src, dst)
}

func (dispatcher Dispatcher) UploadHelp(args []string) {
	fmt.Println("Usage of Upload")
	fmt.Println("\tUpload [SRC] [DST]")
}

func (dispatcher Dispatcher) UploadDesc(args []string) {
	fmt.Println("Upload")
	fmt.Println("\tUpload file from local machine to remote server")
}
