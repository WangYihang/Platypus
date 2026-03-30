package core

import (
	"net"

	"github.com/WangYihang/Platypus/internal/app"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/utils/message"
	"github.com/WangYihang/Platypus/internal/utils/str"
	"github.com/armon/go-socks5"
)

func AddPushTunnelConfig(termite *TermiteClient, local_address string, remote_address string) {
	log.Info("Mapping local (%s) to remote (%s)", local_address, remote_address)

	termite.LockAtom()
	defer termite.UnlockAtom()

	err := termite.Send(message.Message{
		Type: message.PUSH_TUNNEL_CREATE,
		Body: message.BodyPushTunnelCreate{
			Address: remote_address,
		},
	})

	if err != nil {
		log.Error(err.Error())
	} else {
		Ctx.PushTunnelConfig[remote_address] = app.PushTunnelConfig{
			Termite: termite,
			Address: local_address,
		}
	}
}

func AddPullTunnelConfig(termite *TermiteClient, local_address string, remote_address string) {
	log.Info("Mapping remote (%s) to local (%s)", remote_address, local_address)
	tunnel, err := net.Listen("tcp", local_address)
	if err != nil {
		log.Error(err.Error())
		return
	}
	Ctx.PullTunnelConfig[local_address] = app.PullTunnelConfig{
		Termite: termite,
		Address: remote_address,
		Server:  &tunnel,
	}

	go func() {
		for {
			conn, _ := tunnel.Accept()
			token := str.RandomString(0x10)
			err := termite.Send(message.Message{
				Type: message.PULL_TUNNEL_CONNECT,
				Body: message.BodyPullTunnelConnect{
					Token:   token,
					Address: remote_address,
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

	err := termite.Send(message.Message{
		Type: message.PULL_TUNNEL_DATA,
		Body: message.BodyPullTunnelData{
			Token: token,
			Data:  data,
		},
	})
	if err != nil {
		log.Error("Network error: %s", err)
	}
}

func StartSocks5Server(local_address string) error {
	socks5ServerListener, err := net.Listen("tcp", local_address)
	if err != nil {
		return err
	}
	server, err := socks5.New(&socks5.Config{})
	if err != nil {
		return err
	}
	Ctx.Socks5Servers[local_address] = server
	go server.Serve(socks5ServerListener)
	log.Success("Socks server started at: %s", local_address)
	return nil
}
