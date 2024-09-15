package dispatcher

import (
	"fmt"
	"strconv"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/utils/log"
)

func (dispatcher commandDispatcher) REST(args []string) {
	if len(args) != 2 {
		log.Error("Arguments error, use `Help REST` to get more information")
		dispatcher.RESTHelp([]string{})
		return
	}

	host := args[0]
	port, err := strconv.Atoi(args[1])
	if err != nil {
		log.Error("Invalid port: %s, use `Help REST` to get more information", args[1])
		dispatcher.RESTHelp([]string{})
		return
	}

	rest := context.CreateRESTfulAPIServer()
	go rest.Run(fmt.Sprintf("%s:%d", host, port))

	log.Info("RESTful HTTP Server running at %s:%d", host, port)
}

func (dispatcher commandDispatcher) RESTHelp(args []string) {
	fmt.Println("Start a RESTful HTTP Server")
	fmt.Println("\tREST [HOST] [PORT]")
	fmt.Println("\tHOST\tTHe host you want to listen on")
	fmt.Println("\tPORT\tTHe port you want to listen on")
}

func (dispatcher commandDispatcher) RESTDesc(args []string) {
	fmt.Println("REST")
	fmt.Println("\tStart a RESTful HTTP Server to manager all clients")
}
