package core

import (
	"net"

	"github.com/WangYihang/Platypus/internal/app"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/utils/str"
	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
	"github.com/armon/go-socks5"
)

func AddPushTunnelConfig(termite *TermiteClient, localAddress string, remoteAddress string) {
	log.Info("Mapping local (%s) to remote (%s)", localAddress, remoteAddress)

	termite.LockAtom()
	defer termite.UnlockAtom()

	err := termite.Send(&agentpb.Envelope{
		Payload: &agentpb.Envelope_TunnelCreateRequest{
			TunnelCreateRequest: &agentpb.TunnelCreateRequest{
				Address: remoteAddress,
				Mode:    agentpb.TunnelMode_TUNNEL_MODE_PUSH,
			},
		},
	})

	if err != nil {
		log.Error(err.Error())
	} else {
		Ctx.PushTunnelConfig[remoteAddress] = app.PushTunnelConfig{
			Termite: termite,
			Address: localAddress,
		}
	}
}

func AddPullTunnelConfig(termite *TermiteClient, localAddress string, remoteAddress string) {
	log.Info("Mapping remote (%s) to local (%s)", remoteAddress, localAddress)
	tunnel, err := net.Listen("tcp", localAddress)
	if err != nil {
		log.Error(err.Error())
		return
	}
	Ctx.PullTunnelConfig[localAddress] = app.PullTunnelConfig{
		Termite: termite,
		Address: remoteAddress,
		Server:  &tunnel,
	}

	go func() {
		for {
			conn, _ := tunnel.Accept()
			token := str.RandomString(0x10)
			err := termite.Send(&agentpb.Envelope{
				Payload: &agentpb.Envelope_TunnelConnectRequest{
					TunnelConnectRequest: &agentpb.TunnelConnectRequest{
						TunnelId: token,
						Address:  remoteAddress,
					},
				},
			})
			if err == nil {
				Ctx.PullTunnelInstance[token] = app.PullTunnelInstance{
					Conn:    &conn,
					Termite: termite,
				}
			}
		}
	}()
}

func WriteTunnel(termite *TermiteClient, token string, data []byte) {
	termite.LockAtom()
	defer termite.UnlockAtom()

	err := termite.Send(&agentpb.Envelope{
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
	server, err := socks5.New(&socks5.Config{})
	if err != nil {
		return err
	}
	Ctx.Socks5Servers[localAddress] = server
	go server.Serve(listener)
	log.Success("Socks server started at: %s", localAddress)
	return nil
}
