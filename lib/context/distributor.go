package context

import (
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/lib/util/compiler"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/network"
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

		// Step 0: Compile termite.go from template file
		routeKey := c.Param("route_key")
		addr := Ctx.FindServerListeningAddressByDispatchKey(routeKey)
		content, err := ioutil.ReadFile("termite.go")
		if err != nil {
			log.Error("Can not read termite.go: %s", err)
			panicRESTfully(c, "Can not read termite.go")
			return
		}
		contentString := string(content)
		contentString = strings.Replace(contentString, "127.0.0.1:1337", addr, -1)
		err = ioutil.WriteFile("termite.go", []byte(contentString), 0644)
		if err != nil {
			log.Error("Can not write termite.go: %s", err)
			panicRESTfully(c, "Can not write termite.go")
			return
		}

		// Compile termite binary
		target := fmt.Sprintf("build/%s-%s-termite", time.Now().Format("2006-01-02-15:04:05"), str.RandomString(0x10))
		if !compiler.Compile(target) {
			log.Error("Can not compile termite.go: %s", err)
			panicRESTfully(c, "Can not compile termite.go")
			return
		}

		if !compiler.Compile(target) {
			log.Error("Can not compress termite.go: %s", err)
		}

		// Response file
		c.File(target)
	})
	return endpoint
}
