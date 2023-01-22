package listener

import (
	"fmt"
	"net"
)

type TermiteTCPListener struct {
	Listener
	stopChan chan bool
	connChan chan net.Conn
}

func (l *TermiteTCPListener) Enable() (err error) {
	fmt.Println("Enabling Termite TCP Listener")
	return
}

func (l *TermiteTCPListener) Disable() (err error) {
	fmt.Println("Disabling Termite TCP Listener")
	return
}

func (l *TermiteTCPListener) NumClients() int {
	return 2
}

func (l *TermiteTCPListener) Handle(conn net.Conn) {
	conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 28\r\n\r\nthis is termite tcp listener"))
	conn.Close()
}
