package context

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/WangYihang/Platypus/internal/utils/compiler"
	"github.com/WangYihang/Platypus/internal/utils/log"
	"github.com/WangYihang/Platypus/internal/utils/network"
	"github.com/gin-gonic/gin"
)

type Distributor struct {
	Host       string            `json:"host"`
	Port       uint16            `json:"port"`
	Interfaces []string          `json:"interfaces"`
	Route      map[string]string `json:"route"`
	Url        string            `json:"url"`
}

func CreateDistributorServer(host string, port uint16, url string) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = ioutil.Discard
	endpoint := gin.Default()

	// Connect with context
	Ctx.Distributor = &Distributor{
		Host:       host,
		Port:       port,
		Interfaces: network.GatherInterfacesList(host),
		Route:      map[string]string{},
		Url:        url,
	}

	endpoint.GET("/termite/:target", func(c *gin.Context) {
		if !paramsExistOrAbort(c, []string{"target"}) {
			return
		}
		target := c.Param("target")
		// TODO: Check format

		if target == "" {
			log.Error("Invalid connect back addr: %v", target)
			panicRESTfully(c, "Invalid connect back addr")
			return
		}

		// Generate temp folder and filename
		dir, filename, err := compiler.GenerateDirFilename()
		if err != nil {
			log.Error(fmt.Sprint(err))
			panicRESTfully(c, err.Error())
			return
		}
		defer os.RemoveAll(dir)

		// Build Termite binary
		err = compiler.BuildTermiteFromPrebuildAssets(filename, target)
		if err != nil {
			log.Error(fmt.Sprint(err))
			panicRESTfully(c, err.Error())
			return
		}

		// Compress binary
		if !compiler.Compress(filename) {
			log.Error("Can not compress termite.go")
		}

		// Response file
		c.File(filename)
	})
	return endpoint
}
