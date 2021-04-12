package context

import (
	"fmt"
	"io/ioutil"
	"strconv"

	"github.com/gin-gonic/gin"
)

func formExistOrAbort(c *gin.Context, params []string) bool {
	for _, param := range params {
		if c.PostForm(param) == "" {
			return panicRESTfully(c, fmt.Sprintf("%s is required", param))
		}
	}
	return true
}

func paramsExistOrAbort(c *gin.Context, params []string) bool {
	for _, param := range params {
		if c.Param(param) == "" {
			return panicRESTfully(c, fmt.Sprintf("%s is required", param))
		}
	}
	return true
}

func panicRESTfully(c *gin.Context, msg string) bool {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    msg,
	})
	c.Abort()
	return false
}

func CreateRESTfulAPIServer() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = ioutil.Discard
	rest := gin.Default()
	// Server related
	rest.GET("/server", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": true,
			"msg":    Ctx.Servers,
		})
		c.Abort()
		return
	})
	rest.GET("/server/:hash", func(c *gin.Context) {
		if !paramsExistOrAbort(c, []string{"hash"}) {
			return
		}
		hash := c.Param("hash")
		for _, server := range Ctx.Servers {
			if server.Hash() == hash {
				c.JSON(200, gin.H{
					"status": true,
					"msg":    server,
				})
				c.Abort()
				return
			}
		}
		panicRESTfully(c, "No such server")
		return
	})
	rest.GET("/server/:hash/client", func(c *gin.Context) {
		if !paramsExistOrAbort(c, []string{"hash"}) {
			return
		}
		hash := c.Param("hash")
		for _, server := range Ctx.Servers {
			if server.Hash() == hash {
				c.JSON(200, gin.H{
					"status": true,
					"msg":    server.Clients,
				})
				c.Abort()
				return
			}
		}
		panicRESTfully(c, "No such server")
		return
	})
	rest.POST("/server", func(c *gin.Context) {
		if !formExistOrAbort(c, []string{"host", "port"}) {
			return
		}
		port, err := strconv.Atoi(c.Param("port"))
		if err != nil {
			panicRESTfully(c, "Invalid port number")
			return
		}
		hashFormat := "%i %u %m %o"
		server := CreateTCPServer(c.Param("host"), uint16(port), hashFormat)
		go (*server).Run()
		c.JSON(200, gin.H{
			"status": true,
			"msg":    server,
		})
		c.Abort()
		return
	})
	rest.DELETE("/server/:hash", func(c *gin.Context) {
		if !paramsExistOrAbort(c, []string{"hash"}) {
			return
		}
		hash := c.Param("hash")
		for _, server := range Ctx.Servers {
			if server.Hash() == hash {
				Ctx.DeleteServer(server)
				c.JSON(200, gin.H{
					"status": true,
				})
				c.Abort()
				return
			}
		}
		panicRESTfully(c, "No such server")
		return
	})

	// Client related
	rest.GET("/client", func(c *gin.Context) {
		clients := []TCPClient{}
		for _, server := range Ctx.Servers {
			for _, client := range (*server).GetAllTCPClients() {
				clients = append(clients, *client)
			}
		}
		c.JSON(200, gin.H{
			"status": true,
			"msg":    clients,
		})
		c.Abort()
		return
	})
	rest.GET("/client/:hash", func(c *gin.Context) {
		if !paramsExistOrAbort(c, []string{"hash"}) {
			return
		}
		hash := c.Param("hash")
		for _, server := range Ctx.Servers {
			if client, exist := server.Clients[hash]; exist {
				c.JSON(200, gin.H{
					"status": true,
					"msg":    client,
				})
				c.Abort()
				return
			}
		}
		panicRESTfully(c, "No such client")
		return
	})
	rest.DELETE("/client/:hash", func(c *gin.Context) {
		if !paramsExistOrAbort(c, []string{"hash"}) {
			return
		}
		hash := c.Param("hash")
		for _, server := range Ctx.Servers {
			if client, exist := server.Clients[hash]; exist {
				Ctx.DeleteTCPClient(client)
				c.JSON(200, gin.H{
					"status": true,
				})
				c.Abort()
				return
			}
		}
		panicRESTfully(c, "No such client")
		return
	})
	rest.POST("/client/:hash", func(c *gin.Context) {
		if !paramsExistOrAbort(c, []string{"hash"}) {
			return
		}
		if !formExistOrAbort(c, []string{"cmd"}) {
			return
		}
		hash := c.Param("hash")
		cmd := c.PostForm("cmd")
		for _, server := range Ctx.Servers {
			if client, exist := server.Clients[hash]; exist {
				c.JSON(200, gin.H{
					"status": true,
					"msg":    client.SystemToken(cmd),
				})
				c.Abort()
				return
			}
		}
		panicRESTfully(c, "No such client")
		return
	})
	return rest
}
