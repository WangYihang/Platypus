package context

import (
	"io/ioutil"

	"github.com/WangYihang/Platypus/internal/util/network"
	"github.com/gin-contrib/static"
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

	endpoint.Use(static.Serve("/", static.LocalFile("./static", false)))

	return endpoint
}
