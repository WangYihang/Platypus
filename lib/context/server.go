package context

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/lib/util/hash"
	"github.com/WangYihang/Platypus/lib/util/log"
	humanize "github.com/dustin/go-humanize"
)

type TCPServer struct {
	Host      string
	Port      int16
	Clients   map[string](*TCPClient)
	TimeStamp time.Time
}

func CreateTCPServer(host string, port int16) *TCPServer {
	return &TCPServer{
		Host:      host,
		Port:      port,
		Clients:   make(map[string](*TCPClient)),
		TimeStamp: time.Now(),
	}
}

func (s *TCPServer) Hash() string {
	return hash.MD5(fmt.Sprintf("%s:%d:%s", s.Host, s.Port, s.TimeStamp))
}

func (s *TCPServer) Run() {
	service := fmt.Sprintf("%s:%d", s.Host, s.Port)
	tcpAddr, err := net.ResolveTCPAddr("tcp4", service)
	if err != nil {
		log.Error("Resolve TCP address failed: %s", err)
		return
	}
	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		log.Error("Listen failed: %s", err)
		return
	}
	log.Info(fmt.Sprintf("Server running at: %s", s.FullDesc()))

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		client := CreateTCPClient(conn)
		log.Info("New client %s Connected", client.Desc())
		s.AddTCPClient(client)
	}
}

func (s *TCPServer) OnelineDesc() string {
	var buffer bytes.Buffer
	buffer.WriteString(
		fmt.Sprintf(
			"%s:%d (%d online clients)",
			s.Host,
			s.Port,
			len(s.Clients),
		),
	)
	return buffer.String()
}

func (s *TCPServer) FullDesc() string {
	var buffer bytes.Buffer
	buffer.WriteString(
		fmt.Sprintf(
			"[%s] %s:%d (%d online clients) (started at: %s)",
			s.Hash(),
			s.Host,
			s.Port,
			len(s.Clients),
			humanize.Time(s.TimeStamp),
		),
	)
	var descs []string
	for _, client := range s.Clients {
		descs = append(descs, fmt.Sprintf("\t%s", client.Desc()))
	}
	if len(descs) > 0 {
		buffer.WriteString("\n")
	}
	buffer.WriteString(strings.Join(descs, "\n"))
	return buffer.String()
}

func (s *TCPServer) Stop() {
	log.Info(fmt.Sprintf("Stopping server: %s", s.OnelineDesc()))
	for _, client := range s.Clients {
		s.DeleteTCPClient(client)
	}
}

func (s *TCPServer) AddTCPClient(client *TCPClient) {
	s.Clients[client.Hash] = client
}

func (s *TCPServer) DeleteTCPClient(client *TCPClient) {
	client.Close()
	delete(s.Clients, client.Hash)
}

func (s *TCPServer) GetAllTCPClients() map[string](*TCPClient) {
	return s.Clients
}
