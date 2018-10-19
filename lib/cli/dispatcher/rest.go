package dispatcher

import (
	"fmt"
	"strconv"
	"io/ioutil"

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
	rest.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
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
