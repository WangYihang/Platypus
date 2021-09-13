package server

import (
	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/util/validator"
	"github.com/gin-gonic/gin"
)

type ServersWithDistributorAddress struct {
	Servers     map[string](*context.TCPServer) `json:"servers"`
	Distributor context.Distributor             `json:"distributor"`
}

func ListServers(c *gin.Context) {
	response := ServersWithDistributorAddress{
		Servers:     context.Ctx.Servers,
		Distributor: *context.Ctx.Distributor,
	}
	c.JSON(200, gin.H{
		"status": true,
		"msg":    response,
	})
	c.Abort()
}

func GetServerInfo(c *gin.Context) {
	if !validator.ParamsExistOrAbort(c, []string{"hash"}) {
		return
	}
	hash := c.Param("hash")
	for _, server := range context.Ctx.Servers {
		if server.Hash == hash {
			c.JSON(200, gin.H{
				"status": true,
				"msg":    server,
			})
			c.Abort()
			return
		}
	}
	validator.PanicRESTfully(c, "No such server")
}

func GetServerClients(c *gin.Context) {
	if !validator.ParamsExistOrAbort(c, []string{"hash"}) {
		return
	}
	hash := c.Param("hash")
	for _, server := range context.Ctx.Servers {
		if server.Hash == hash {
			clients := make(map[string]interface{})
			for k, v := range server.Clients {
				clients[k] = v
			}
			for k, v := range server.TermiteClients {
				clients[k] = v
			}
			c.JSON(200, gin.H{
				"status": true,
				"msg":    clients,
			})
			c.Abort()
			return
		}
	}
	validator.PanicRESTfully(c, "No such server")
}
