package api

import (
	"fmt"
	"strconv"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/gin-gonic/gin"
)

// RegisterLegacyRoutes wires up the pre-v1 /api/server and /api/client routes
// behind the same Bearer auth as /api/v1/*. These remain in use by platypus-admin
// CLI; consolidating them under /api/v1/ is tracked in the modernization plan.
func RegisterLegacyRoutes(engine *gin.Engine, auth *Auth) {
	g := engine.Group("/api")
	g.Use(auth.Middleware())

	servers := g.Group("/server")
	servers.GET("", ListServers)
	servers.GET("/:hash", GetServer)
	servers.GET("/:hash/client", GetServerClients)
	servers.POST("", CreateServer)
	servers.DELETE("/:hash", DeleteServer)

	clients := g.Group("/client")
	clients.GET("", ListClients)
	clients.GET("/:hash", GetClient)
	clients.GET("/:hash/upgrade/:target", UpgradeClient)
	clients.DELETE("/:hash", DeleteClient)
	clients.POST("/:hash", ExecClient)
}

type serversWithDistributor struct {
	Servers     map[string]*core.TCPServer `json:"servers"`
	Distributor core.Distributor           `json:"distributor"`
}

// ListServers returns every TCP listener plus the distributor snapshot.
func ListServers(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": true,
		"msg": serversWithDistributor{
			Servers:     core.GetServers(),
			Distributor: *core.Ctx.Distributor.(*core.Distributor),
		},
	})
	c.Abort()
}

// GetServer fetches one TCP listener by hash.
func GetServer(c *gin.Context) {
	if !paramsExistOrAbort(c, []string{"hash"}) {
		return
	}
	hash := c.Param("hash")
	for _, server := range core.GetServers() {
		if server.Hash == hash {
			c.JSON(200, gin.H{"status": true, "msg": server})
			c.Abort()
			return
		}
	}
	panicRESTfully(c, "No such server")
}

// GetServerClients lists every TCP/Termite client attached to a listener.
func GetServerClients(c *gin.Context) {
	if !paramsExistOrAbort(c, []string{"hash"}) {
		return
	}
	hash := c.Param("hash")
	for _, server := range core.GetServers() {
		if server.Hash == hash {
			clients := make(map[string]interface{})
			for k, v := range server.Clients {
				clients[k] = v
			}
			for k, v := range server.TermiteClients {
				clients[k] = v
			}
			c.JSON(200, gin.H{"status": true, "msg": clients})
			c.Abort()
			return
		}
	}
	panicRESTfully(c, "No such server")
}

// CreateServer starts a new TCP listener.
func CreateServer(c *gin.Context) {
	if !formExistOrAbort(c, []string{"host", "port", "encrypted"}) {
		return
	}
	port, err := strconv.Atoi(c.PostForm("port"))
	if err != nil || port <= 0 || port > 65535 {
		panicRESTfully(c, "Invalid port number")
		return
	}
	encrypted, _ := strconv.ParseBool(c.PostForm("encrypted"))
	server := core.CreateTCPServer(c.PostForm("host"), uint16(port), "", encrypted, true, "", "")
	if server == nil {
		c.JSON(200, gin.H{
			"status": false,
			"msg":    fmt.Sprintf("The server (%s:%d) start failed", c.PostForm("host"), port),
		})
		c.Abort()
		return
	}
	go (*server).Run()
	c.JSON(200, gin.H{"status": true, "msg": server})
	c.Abort()
}

// DeleteServer tears down a TCP listener by hash.
func DeleteServer(c *gin.Context) {
	if !paramsExistOrAbort(c, []string{"hash"}) {
		return
	}
	hash := c.Param("hash")
	for _, server := range core.GetServers() {
		if server.Hash == hash {
			core.DeleteServer(server)
			c.JSON(200, gin.H{"status": true})
			c.Abort()
			return
		}
	}
	panicRESTfully(c, "No such server")
}

// ListClients returns every connected client across every listener.
func ListClients(c *gin.Context) {
	clients := make(map[string]interface{})
	for _, server := range core.GetServers() {
		for k, v := range server.Clients {
			clients[k] = v
		}
		for k, v := range server.TermiteClients {
			clients[k] = v
		}
	}
	c.JSON(200, gin.H{"status": true, "msg": clients})
	c.Abort()
}

// GetClient fetches one plain TCP client by hash.
func GetClient(c *gin.Context) {
	if !paramsExistOrAbort(c, []string{"hash"}) {
		return
	}
	hash := c.Param("hash")
	for _, server := range core.GetServers() {
		if client, exist := server.Clients[hash]; exist {
			c.JSON(200, gin.H{"status": true, "msg": client})
			c.Abort()
			return
		}
	}
	panicRESTfully(c, "No such client")
}

// UpgradeClient asks the server to compile + push a Termite agent to the
// target listener, replacing a plain reverse shell with an encrypted one.
func UpgradeClient(c *gin.Context) {
	if !paramsExistOrAbort(c, []string{"hash", "target"}) {
		return
	}
	hash := c.Param("hash")
	target := c.Param("target")
	if target == "" {
		panicRESTfully(c, "Invalid server hash")
		return
	}
	client := core.FindTCPClientByHash(hash)
	if client == nil {
		panicRESTfully(c, "No such client")
		return
	}
	go client.UpgradeToTermite(target)
	c.JSON(200, gin.H{
		"status": true,
		"msg":    fmt.Sprintf("Upgrading client %s to termite", client.OnelineDesc()),
	})
	c.Abort()
}

// DeleteClient disconnects a TCP client by hash.
func DeleteClient(c *gin.Context) {
	if !paramsExistOrAbort(c, []string{"hash"}) {
		return
	}
	hash := c.Param("hash")
	for _, server := range core.GetServers() {
		if client, exist := server.Clients[hash]; exist {
			core.DeleteTCPClient(client)
			c.JSON(200, gin.H{"status": true})
			c.Abort()
			return
		}
	}
	panicRESTfully(c, "No such client")
}

// ExecClient executes a command on a TCP or Termite client and returns the output.
func ExecClient(c *gin.Context) {
	if !paramsExistOrAbort(c, []string{"hash"}) {
		return
	}
	if !formExistOrAbort(c, []string{"cmd"}) {
		return
	}
	hash := c.Param("hash")
	cmd := c.PostForm("cmd")
	for _, server := range core.GetServers() {
		if client, exist := server.Clients[hash]; exist {
			if client.GetPtyEstablished() {
				c.JSON(200, gin.H{
					"status": false,
					"msg":    "The client is under PTY mode, please exit pty mode before execute command on it",
				})
			} else {
				c.JSON(200, gin.H{"status": true, "msg": client.SystemToken(cmd)})
			}
			c.Abort()
			return
		}
		if client, exist := server.TermiteClients[hash]; exist {
			c.JSON(200, gin.H{"status": true, "msg": client.System(cmd)})
			c.Abort()
			return
		}
	}
	panicRESTfully(c, "No such client")
}
