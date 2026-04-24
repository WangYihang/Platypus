package core

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/phayes/freeport"

	"github.com/WangYihang/Platypus/internal/app"
	"github.com/WangYihang/Platypus/internal/log"
	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// WebSocketMessage is the JSON envelope that client-lifecycle notify
// broadcasts are sent over. Retained for wire-format stability with
// existing browsers that still key off WebSocketMessageType.
type WebSocketMessage struct {
	Type WebSocketMessageType
	Data interface{}
}

type WebSocketMessageType int

const (
	CLIENT_CONNECTED WebSocketMessageType = iota
	CLIENT_DUPLICATED
	SERVER_DUPLICATED
)

// handleAgentConnection is the shared post-accept pipeline every agent
// connection runs through once the ingress dispatcher has given us a
// live net.Conn speaking the protobuf protocol. Enrollment →
// GatherClientInfo → host + session persistence → service
// registration → message-dispatch goroutine.
func handleAgentConnection(client *AgentClient) {
	// Optional enrollment handshake. If the agent was built with
	// credential support it sends AgentEnrollRequest first; we redeem
	// and reply with AgentEnrollResponse. Legacy agents stay silent
	// for enrollWaitTimeout and we proceed straight to the old
	// GatherClientInfo flow.
	enroll, err := TryEnroll(client)
	if err != nil {
		log.Warn("Enrollment error from %s: %s", client.conn.RemoteAddr(), err)
		client.Close()
		return
	}
	if enroll.Attempted && !enroll.Succeeded {
		log.Warn("Rejected agent from %s (enrollment outcome=%s)",
			client.conn.RemoteAddr(), enroll.Outcome)
		client.Close()
		return
	}
	if enroll.Succeeded {
		log.Info("Agent enrolled from %s (agent_id=%s)",
			client.conn.RemoteAddr(), enroll.AgentID)
		if enroll.ProjectID != "" {
			client.ProjectID = enroll.ProjectID
		}
	}

	log.Info("Gathering information from client...")
	if !client.GatherClientInfo(client.HashFormat) {
		log.Info("Failed to check encrypted income connection from %s", client.conn.RemoteAddr())
		client.Close()
		return
	}

	log.Info("Agent (v%s) connected from %s", client.Version, client.conn.RemoteAddr())
	ctx := context.Background()
	UpsertHostForAgent(ctx, client)
	PersistSessionForAgent(ctx, client)
	if agentSvc != nil {
		agentSvc.addClient(client)
	} else {
		// Without a service registered the connection has nowhere to
		// live — close instead of leaking the goroutine.
		log.Warn("no AgentService registered; dropping %s", client.conn.RemoteAddr())
		client.Close()
		return
	}
	recordSessionOpen(client)
}

// AgentMessageDispatcher is the per-connection read loop. Every
// envelope the agent sends after enrollment is routed here; payload
// types split across process control, tunnel I/O, file / exec RPC
// responses, and the in-band session renewal handshake. The function
// runs on its own goroutine per agent and returns when Recv fails.
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
			AddPullTunnelConfig(client, localAddr, remoteAddr)
		case *agentpb.Envelope_Socks5CreateFailed:
			log.Error("%s", p.Socks5CreateFailed.Reason)

		case *agentpb.Envelope_SysInfoResponse:
			if p.SysInfoResponse != nil {
				PutSysInfo(client.Hash, p.SysInfoResponse.Info)
			}

		// RPC responses — route to EnvelopeQueue by request_id
		case *agentpb.Envelope_ExecResponse,
			*agentpb.Envelope_ReadFileResponse,
			*agentpb.Envelope_FileSizeResponse,
			*agentpb.Envelope_WriteFileResponse,
			*agentpb.Envelope_ListDirResponse,
			*agentpb.Envelope_StatResponse,
			*agentpb.Envelope_DeleteResponse,
			*agentpb.Envelope_RenameResponse,
			*agentpb.Envelope_MkdirResponse,
			*agentpb.Envelope_ChmodResponse:
			token := env.RequestId
			Ctx.MessageQueueMu.RLock()
			ch, exists := Ctx.EnvelopeQueue[token]
			Ctx.MessageQueueMu.RUnlock()
			if exists {
				ch <- env
			} else {
				log.Error("No such channel: %s", token)
			}

		case *agentpb.Envelope_SessionRenewRequest:
			handleSessionRenew(client, env.RequestId, p.SessionRenewRequest)
		}
	}
}
