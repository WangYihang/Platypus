package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"time"
)

type client struct {
	addr net.Addr
	ts   time.Time
}

var clients map[string]client

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
}

func handleClient(conn net.Conn) {
	defer conn.Close()
	daytime := time.Now().String()
	conn.Write([]byte(daytime))
}

func startServer(host string, port int16) {
	service := fmt.Sprintf("%s:%d", host, port)
	tcpAddr, err := net.ResolveTCPAddr("tcp4", service)
	if err != nil {
		fmt.Println(err)
		return
	}
	listener, err := net.ListenTCP("tcp", tcpAddr)
	fmt.Println("Server running at: ", service)
	if err != nil {
		fmt.Println(err)
		return
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go handleClient(conn)
	}
}

func commandDispatcher() {
	inputReader := bufio.NewReader(os.Stdin)
	for {
		fmt.Println("Please enter some input: ")
		input, err := inputReader.ReadString('\n')
		if err == nil {
			fmt.Printf("The input was: %s\n", input)
		}
		exitFlag := false
		switch input {
		case "exit\n":
			fmt.Println("exiting...")
			exitFlag = true
			break
		case "run\n":
			go startServer("0.0.0.0", 4444)
			break
		default:
			fmt.Println("Unsupported command: ", input)
		}
		if exitFlag {
			break
		}
	}
}

func main() {
	commandDispatcher()
}
