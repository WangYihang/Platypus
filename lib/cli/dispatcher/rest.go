package dispatcher

import (
	"fmt"
	"strconv"
	"io/ioutil"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/gin-gonic/gin"
)

func (dispatcher Dispatcher) REST(args []string) {
	if len(args) != 2 {
		log.Error("Argments error, use `Help REST` to get more information")
		dispatcher.RunHelp([]string{})
		return
	}

	host := args[0]
	port, err := strconv.ParseInt(args[1], 10, 32)
	if err != nil {
		log.Error("Invalid port: %s, use `Help REST` to get more information", args[1])
		dispatcher.RunHelp([]string{})
		return
	}

	// TODO:
	// Add command to disable/enable Gin output
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = ioutil.Discard
	rest := gin.Default()
	rest.GET("/client", func(c *gin.Context) {
		clients := []string{} 
		for _, server := range context.Ctx.Servers {
			for _, client := range (*server).GetAllTCPClients() {
				clients = append(clients, client.Conn.RemoteAddr().String())
			}
		}
		c.JSON(200, gin.H{
			"status": true,
			"msg": clients,
		})
	})
	rest.POST("/client/:hash", func(c *gin.Context) {
		hash := c.Param("hash")
		cmd := c.PostForm("cmd")
		response := "No such client"
		flag := false
		for _, server := range context.Ctx.Servers {
			for _, client := range (*server).GetAllTCPClients() {
				if hash == client.Hash {
					response =  client.SystemToken(cmd)
					flag = true
				}
			}
		}
		c.JSON(200, gin.H{
			"status": flag,
			"msg": response,
		})
	})
	go rest.Run(fmt.Sprintf("%s:%d", host, port))
	log.Info("RESTful HTTP Server running at %s:%d", host, port)
}

func (dispatcher Dispatcher) RESTHelp(args []string) {
	fmt.Println("Start a RESTful HTTP Server")
	fmt.Println("\tREST [HOST] [PORT]")
	fmt.Println("\tHOST\tTHe host you want to listen on")
	fmt.Println("\tPORT\tTHe port you want to listen on")
}

func (dispatcher Dispatcher) RESTDesc(args []string) {
	fmt.Println("REST")
	fmt.Println("\tStart a RESTful HTTP Server to manager all clients")
}
