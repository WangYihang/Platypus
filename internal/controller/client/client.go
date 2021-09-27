package client_controller

import (
	"fmt"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/util/validator"
	"github.com/gin-gonic/gin"
)

func GetAllClients(c *gin.Context) {
	clients := make(map[string]interface{})
	for _, server := range context.Ctx.Servers {
		for k, v := range server.Clients {
			clients[k] = v
		}
		for k, v := range server.TermiteClients {
			clients[k] = v
		}
	}
	c.JSON(200, gin.H{
		"status": true,
		"msg":    clients,
	})
	c.Abort()
}

func GetClient(c *gin.Context) {
	if !validator.ParamsExistOrAbort(c, []string{"hash"}) {
		return
	}
	hash := c.Param("hash")
	for _, server := range context.Ctx.Servers {
		if client, exist := server.Clients[hash]; exist {
			c.JSON(200, gin.H{
				"status": true,
				"msg":    client,
			})
			c.Abort()
			return
		}
	}
	validator.PanicRESTfully(c, "No such client")
}

func UpgradeToTermite(c *gin.Context) {
	if !validator.ParamsExistOrAbort(c, []string{"hash", "target"}) {
		return
	}
	hash := c.Param("hash")
	target := c.Param("target")
	// TODO: Check target format
	if target == "" {
		validator.PanicRESTfully(c, "Invalid server hash")
		return
	}

	client := context.Ctx.FindTCPClientByHash(hash)
	if client != nil {
		// Upgrade
		go client.UpgradeToTermite(target)
		c.JSON(200, gin.H{
			"status": true,
			"msg":    fmt.Sprintf("Upgrading client %s to termite", client.OnelineDesc()),
		})
		c.Abort()
		return
	}

	validator.PanicRESTfully(c, "No such client")
}

func DeleteClient(c *gin.Context) {
	if !validator.ParamsExistOrAbort(c, []string{"hash"}) {
		return
	}
	hash := c.Param("hash")
	for _, server := range context.Ctx.Servers {
		if client, exist := server.Clients[hash]; exist {
			context.Ctx.DeleteTCPClient(client)
			c.JSON(200, gin.H{
				"status": true,
			})
			c.Abort()
			return
		}
	}
	validator.PanicRESTfully(c, "No such client")
}

func ExecuteCommand(c *gin.Context) {
	if !validator.ParamsExistOrAbort(c, []string{"hash"}) {
		return
	}
	if !validator.FormExistOrAbort(c, []string{"cmd"}) {
		return
	}
	hash := c.Param("hash")
	cmd := c.PostForm("cmd")
	for _, server := range context.Ctx.Servers {
		if client, exist := server.Clients[hash]; exist {
			if client.GetPtyEstablished() {
				c.JSON(200, gin.H{
					"status": false,
					"msg":    "The client is under PTY mode, please exit pty mode before execute command on it",
				})
			} else {
				c.JSON(200, gin.H{
					"status": true,
					"msg":    client.SystemToken(cmd),
				})
			}
			c.Abort()
			return
		}
		if client, exist := server.TermiteClients[hash]; exist {
			c.JSON(200, gin.H{
				"status": true,
				"msg":    client.System(cmd),
			})
			c.Abort()
			return
		}
	}
	validator.PanicRESTfully(c, "No such client")
}

func CollectClientInfo(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}

func GetAllProxies(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}

func CreateProxy(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}

func DeleteProxy(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}

func StartProxy(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}

func StopProxy(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}

func LibReadDir(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}

func LibStat(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}

func LibReadFile(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}

func LibWriteFile(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}

func LibFopen(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}

func LibFseek(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}

func LibFread(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}

func LibFwrite(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}

func LibFclose(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}

func InstallCrontab(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}

func InstallSshKey(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    "To be implemented",
	})
}
