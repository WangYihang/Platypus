package core

import (
	"net"

	socks5 "github.com/things-go/go-socks5"

	"github.com/WangYihang/Platypus/internal/app"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/utils/str"
	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

func AddPushTunnelConfig(agent *AgentClient, localAddress string, remoteAddress string) {
	log.Info("Mapping local (%s) to remote (%s)", localAddress, remoteAddress)

	agent.LockAtom()
	defer agent.UnlockAtom()

	err := agent.Send(&agentpb.Envelope{
		Payload: &agentpb.Envelope_TunnelCreateRequest{
			TunnelCreateRequest: &agentpb.TunnelCreateRequest{
				Address: remoteAddress,
				Mode:    agentpb.TunnelMode_TUNNEL_MODE_PUSH,
			},
		},
	})

	if err != nil {
		log.Error("%s", err.Error())
	} else {
		Ctx.PushTunnelConfig[remoteAddress] = app.PushTunnelConfig{
			Agent:   agent,
			Address: localAddress,
		}
	}
}

func AddPullTunnelConfig(agent *AgentClient, localAddress string, remoteAddress string) {
	log.Info("Mapping remote (%s) to local (%s)", remoteAddress, localAddress)
	tunnel, err := net.Listen("tcp", localAddress)
	if err != nil {
		log.Error("%s", err.Error())
		return
	}
	Ctx.PullTunnelConfig[localAddress] = app.PullTunnelConfig{
		Agent:   agent,
		Address: remoteAddress,
		Server:  &tunnel,
	}

	go func() {
		for {
			conn, _ := tunnel.Accept()
			token := str.RandomString(0x10)
			err := agent.Send(&agentpb.Envelope{
				Payload: &agentpb.Envelope_TunnelConnectRequest{
					TunnelConnectRequest: &agentpb.TunnelConnectRequest{
						TunnelId: token,
						Address:  remoteAddress,
					},
				},
			})
			if err == nil {
				Ctx.PullTunnelInstance[token] = app.PullTunnelInstance{
					Conn:  &conn,
					Agent: agent,
				}
			}
		}
	}()
}

func WriteTunnel(agent *AgentClient, token string, data []byte) {
	agent.LockAtom()
	defer agent.UnlockAtom()

	err := agent.Send(&agentpb.Envelope{
		Payload: &agentpb.Envelope_TunnelData{
			TunnelData: &agentpb.TunnelData{
				TunnelId: token,
				Data:     data,
			},
		},
	})
	if err != nil {
		log.Error("Network error: %s", err)
	}
}

func StartSocks5Server(localAddress string) error {
	listener, err := net.Listen("tcp", localAddress)
	if err != nil {
		return err
	}
	server := socks5.NewServer()
	Ctx.Socks5Servers[localAddress] = server
	go server.Serve(listener)
	log.Success("Socks server started at: %s", localAddress)
	return nil
}
