package cmd

import (
	"os"
	"time"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
	oss "github.com/WangYihang/Platypus/internal/utils/os"
	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v6"
	"github.com/vbauerster/mpb/v6/decor"
)

var uploadCmd = &cobra.Command{
	Use:   "Upload [SRC] [DST]",
	Short: "Upload a file to the current session",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		if core.Ctx.Current == nil && core.Ctx.CurrentTermite == nil {
			log.Error("The current client is not set, please use `Jump` to set it")
			return
		}
		src := args[0]
		dst := args[1]

		if core.Ctx.Current != nil {
			if core.Ctx.Current.OS == oss.Windows {
				log.Error("Upload command does not support Windows platform")
				return
			}
			core.Ctx.Current.Upload(src, dst, false)
			log.Success("File %s uploaded to %s", src, dst)
			return
		}

		if core.Ctx.CurrentTermite != nil {
			log.Info("Uploading %s to %s from client: %s", src, dst, core.Ctx.CurrentTermite.OnelineDesc())
			srcfd, err := os.OpenFile(src, os.O_RDONLY, 0644)
			if err != nil {
				log.Error(err.Error())
				return
			}
			fi, _ := srcfd.Stat()
			totalBytes := fi.Size()

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
			buffer := make([]byte, blockSize)
			for i := int64(0); i < totalBytes; i += blockSize {
				start := time.Now()
				n, err := srcfd.Read(buffer)
				if err != nil {
					bar.Abort(true)
					log.Error("%s", err)
					return
				}
				if n, err = core.Ctx.CurrentTermite.WriteFileEx(dst, buffer[0:n]); err != nil {
					log.Error("Failed to write data: %s", err)
					bar.Abort(true)
					return
				}
				bar.IncrBy(n)
				bar.DecoratorEwmaUpdate(time.Since(start))
			}
			p.Wait()
		}
	},
}
