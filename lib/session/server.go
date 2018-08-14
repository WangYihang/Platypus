package session

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"time"
)

type Server struct {
	host    string
	port    int16
	clients map[string](*Client)
	ts      time.Time
}

var server *Server

func CreateServer(host string, port int16) *Server {
	return &Server{
		host:    host,
		port:    port,
		clients: make(map[string](*Client)),
		ts:      time.Now(),
	}
}

func (s Server) Listen() (*net.TCPListener, error) {
	service := fmt.Sprintf("%s:%d", s.host, s.port)
	tcpAddr, err := net.ResolveTCPAddr("tcp4", service)
	if err != nil {
		return nil, err
	}
	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return nil, err
	}
	fmt.Println("Server running at: ", s.Desc())
	return listener, nil
}

func (s Server) Run(listener *net.TCPListener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		client := CreateClient(conn)
		fmt.Println(client.Desc())
		go s.AddClient(client)
	}
}

func (s Server) Desc() string {
	var buffer bytes.Buffer
	buffer.WriteString(
		fmt.Sprintf(
			"[%s] %s:%d (%d online clients)",
			s.ts.Format("2006-01-02 15:04:05"),
			s.host,
			s.port,
			len(s.clients),
		),
	)
	var descs []string
	for hash, client := range s.clients {
		descs = append(descs, fmt.Sprintf("\t%s (%s)", client.Desc(), hash))
	}
	if len(descs) > 0 {
		buffer.WriteString("\n")
	}
	buffer.WriteString(strings.Join(descs, "\n"))
	return buffer.String()
}

func (s Server) Stop() {
	// TODO
}

func (s Server) AddClient(client *Client) {
	s.clients[client.Hash()] = client
}

func (s Server) DeleteClient(client Client) {
	delete(s.clients, client.Desc())
}

func (s Server) GetAllClients() map[string](*Client) {
	return s.clients
}

func (s Server) Cleanup() {
	for _, client := range s.clients {
		client.conn.Close()
	}
}
