package core

import (
	"fmt"
	"io"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/utils/compiler"
	"github.com/WangYihang/Platypus/internal/utils/network"
)

func distributorParamsExist(c *gin.Context, params []string) bool {
	for _, param := range params {
		if c.Param(param) == "" {
			c.JSON(200, gin.H{"status": false, "msg": fmt.Sprintf("%s is required", param)})
			c.Abort()
			return false
		}
	}
	return true
}

func distributorPanic(c *gin.Context, msg string) {
	c.JSON(200, gin.H{"status": false, "msg": msg})
	c.Abort()
}

type Distributor struct {
	Host       string            `json:"host"`
	Port       uint16            `json:"port"`
	Interfaces []string          `json:"interfaces"`
	Route      map[string]string `json:"route"`
	Url        string            `json:"url"`
}

// CreateDistributorServer returns a gin engine that serves on-demand agent
// binaries built for the requested connect-back target. Admins download the
// agent from here to install on a managed host.
func CreateDistributorServer(host string, port uint16, url string) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = io.Discard
	endpoint := gin.Default()

	// Connect with context
	Ctx.Distributor = &Distributor{
		Host:       host,
		Port:       port,
		Interfaces: network.GatherInterfacesList(host),
		Route:      map[string]string{},
		Url:        url,
	}

	endpoint.GET("/agent/:target", func(c *gin.Context) {
		if !distributorParamsExist(c, []string{"target"}) {
			return
		}
		target := c.Param("target")

		if target == "" {
			log.Error("Invalid connect back addr: %v", target)
			distributorPanic(c, "Invalid connect back addr")
			return
		}

		dir, filename, err := compiler.GenerateDirFilename()
		if err != nil {
			log.Error("%s", err)
			distributorPanic(c, err.Error())
			return
		}
		defer os.RemoveAll(dir)

		err = compiler.BuildAgentFromPrebuildAssets(filename, target)
		if err != nil {
			log.Error("%s", err)
			distributorPanic(c, err.Error())
			return
		}

		if !compiler.Compress(filename) {
			log.Error("Can not compress agent binary")
		}

		c.File(filename)
	})
	return endpoint
}
