package server_model

import (
	"fmt"
	"strconv"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/util/validator"
	"github.com/gin-gonic/gin"
)

type ServersWithDistributorAddress struct {
	Servers     map[string](*context.TCPServer) `json:"servers"`
	Distributor context.Distributor             `json:"distributor"`
}

func GetAllServers(c *gin.Context) {
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

func GetServer(c *gin.Context) {
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

func GetAllClientsOfServer(c *gin.Context) {
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

func CreateServer(c *gin.Context) {
	if !validator.FormExistOrAbort(c, []string{"host", "port", "encrypted"}) {
		return
	}
	port, err := strconv.Atoi(c.PostForm("port"))
	if err != nil || port <= 0 || port > 65535 {
		validator.PanicRESTfully(c, "Invalid port number")
		return
	}
	encrypted, _ := strconv.ParseBool(c.PostForm("encrypted"))
	server := context.CreateTCPServer(c.PostForm("host"), uint16(port), "", encrypted, true, "")
	if server != nil {
		go (*server).Run()
		c.JSON(200, gin.H{
			"status": true,
			"msg":    server,
		})
		c.Abort()
	} else {
		c.JSON(200, gin.H{
			"status": false,
			"msg":    fmt.Sprintf("The server (%s:%d) start failed", c.PostForm("host"), port),
		})
		c.Abort()
	}
}

func DeleteServer(c *gin.Context) {
	if !validator.ParamsExistOrAbort(c, []string{"hash"}) {
		return
	}
	hash := c.Param("hash")
	for _, server := range context.Ctx.Servers {
		if server.Hash == hash {
			context.Ctx.DeleteServer(server)
			c.JSON(200, gin.H{
				"status": true,
			})
			c.Abort()
			return
		}
	}
	validator.PanicRESTfully(c, "No such server")
}

func StartServer(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}

func StopServer(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}
