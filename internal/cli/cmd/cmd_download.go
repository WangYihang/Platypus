package cmd

import (
	"os"
	"time"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/utils/ui"
	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v6"
	"github.com/vbauerster/mpb/v6/decor"
)

var downloadCmd = &cobra.Command{
	Use:   "Download [SRC] [DST]",
	Short: "Download a file from the current session",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		if core.Ctx.Current == nil && core.Ctx.CurrentTermite == nil {
			log.Error("The current client is not set, please use `Jump` to set it")
			return
		}

		src := args[0]
		dst := args[1]

		if _, err := os.Stat(dst); err == nil {
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

		if core.Ctx.Current != nil {
			log.Info("Downloading %s to %s from client: %s", src, dst, core.Ctx.Current.(*core.TCPClient).OnelineDesc())
			totalBytes, err := core.Ctx.Current.(*core.TCPClient).FileSize(src)
			if err != nil {
				log.Error("Failed to get file size: %s", err)
				return
			}
			log.Info("Filesize: %d bytes", totalBytes)

			p := mpb.New(mpb.WithWidth(64))
			bar := p.Add(int64(totalBytes), mpb.NewBarFiller("[=>-|"),
				mpb.PrependDecorators(decor.CountersKibiByte("% .2f / % .2f")),
				mpb.AppendDecorators(
					decor.EwmaETA(decor.ET_STYLE_HHMMSS, 60),
					decor.Name(" ] "),
					decor.EwmaSpeed(decor.UnitKB, "% .2f", 60),
				),
			)

			blockSize := 0x1000
			firstBlockSize := totalBytes % blockSize

			start := time.Now()
			content, err := core.Ctx.Current.(*core.TCPClient).ReadFileEx(src, 0, firstBlockSize)
			if err != nil {
				bar.Abort(true)
				log.Error("%s", err)
				return
			}
			n, err := dstfd.Write([]byte(content))
			if err != nil {
				bar.Abort(true)
				log.Error("Failed to write data: %s", err)
				return
			}
			bar.IncrBy(n)
			bar.DecoratorEwmaUpdate(time.Since(start))

			for i := 0; i < totalBytes/blockSize; i++ {
				start = time.Now()
				content, err := core.Ctx.Current.(*core.TCPClient).ReadFileEx(src, firstBlockSize+i*blockSize, blockSize)
				if err != nil {
					bar.Abort(true)
					log.Error("%s", err)
					return
				}
				n, err = dstfd.Write([]byte(content))
				if err != nil {
					bar.Abort(true)
					log.Error("Failed to write data: %s", err)
					return
				}
				bar.IncrBy(n)
				bar.DecoratorEwmaUpdate(time.Since(start))
			}
			p.Wait()
			return
		}

		if core.Ctx.CurrentTermite != nil {
			log.Info("Downloading %s to %s from client: %s", src, dst, core.Ctx.CurrentTermite.(*core.TermiteClient).OnelineDesc())
			totalBytes, err := core.Ctx.CurrentTermite.(*core.TermiteClient).FileSize(src)
			if err != nil {
				log.Error("Failed to get file size: %s", err)
				return
			}
			log.Info("Filesize: %d bytes", totalBytes)

			p := mpb.New(mpb.WithWidth(64))
			bar := p.Add(totalBytes, mpb.NewBarFiller("[=>-|"),
				mpb.PrependDecorators(decor.CountersKibiByte("% .2f / % .2f")),
				mpb.AppendDecorators(
					decor.EwmaETA(decor.ET_STYLE_HHMMSS, 60),
					decor.Name(" ] "),
					decor.EwmaSpeed(decor.UnitKB, "% .2f", 60),
				),
			)

			blockSize := int64(0x400 * 512)
			for i := int64(0); i < totalBytes; i += blockSize {
				start := time.Now()
				content, err := core.Ctx.CurrentTermite.(*core.TermiteClient).ReadFileEx(src, i, blockSize)
				if err != nil {
					log.Error(err.Error())
					return
				}
				n, err := dstfd.Write(content)
				if err != nil {
					log.Error("Failed to write data: %s", err)
					return
				}
				bar.IncrBy(n)
				bar.DecoratorEwmaUpdate(time.Since(start))
			}
			p.Wait()
		}
	},
}
