package core

import (
	"fmt"
	"io"
	"os"

	"github.com/WangYihang/Platypus/internal/utils/compiler"
	"github.com/WangYihang/Platypus/internal/utils/log"
	"github.com/WangYihang/Platypus/internal/utils/network"
	"github.com/gin-gonic/gin"
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

	endpoint.GET("/termite/:target", func(c *gin.Context) {
		if !distributorParamsExist(c, []string{"target"}) {
			return
		}
		target := c.Param("target")
		// TODO: Check format

		if target == "" {
			log.Error("Invalid connect back addr: %v", target)
			distributorPanic(c,"Invalid connect back addr")
			return
		}

		// Generate temp folder and filename
		dir, filename, err := compiler.GenerateDirFilename()
		if err != nil {
			log.Error(fmt.Sprint(err))
			distributorPanic(c,err.Error())
			return
		}
		defer os.RemoveAll(dir)

		// Build Termite binary
		err = compiler.BuildTermiteFromPrebuildAssets(filename, target)
		if err != nil {
			log.Error(fmt.Sprint(err))
			distributorPanic(c,err.Error())
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
