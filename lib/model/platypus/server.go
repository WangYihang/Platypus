package platypus

import (
	"fmt"
	"net"
	"time"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/hash"
	"github.com/WangYihang/Platypus/lib/util/log"
)

type PlatypusServer struct {
	context.TCPServer
}

func CreatePlatypusServer(host string, port int16) *PlatypusServer {
	ts := time.Now()
	return &PlatypusServer{
		context.TCPServer{
			Name:      "Platypus",
			Host:      host,
			Port:      port,
			Clients:   make(map[string](*context.TCPClient)),
			TimeStamp: ts,
			Hash:      hash.MD5(fmt.Sprintf("%s:%s:%s", host, port, ts)),
		},
	}
}

func (s *PlatypusServer) Run() {
	log.Info("Platypus server running...")
	service := fmt.Sprintf("%s:%d", s.Host, s.Port)
	tcpAddr, err := net.ResolveTCPAddr("tcp4", service)
	if err != nil {
		log.Error("Resolve TCP address failed: ", err)
		return
	}
	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		log.Error("Listen failed: ", err)
		return
	}
	log.Info(fmt.Sprintf("Server running at: %s", s.FullDesc()))

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		client := CreatePlatypusClient(conn)
		log.Info("New client %s Connected", client.OnelineDesc())
		if client.Auth() {
			log.Warn("Client %s auth succeed", client.OnelineDesc())
			s.AddPlatypusClient(&client.TCPClient)
		} else {
			log.Warn("Client %s auth failed, connection reseted by server", client.OnelineDesc())
			client.Close()
		}
	}
}

func (s *PlatypusServer) AddPlatypusClient(client *context.TCPClient) {
	s.Clients[client.Hash] = client
}

func (s *PlatypusServer) DeletePlatypusClient(client *context.TCPClient) {
	client.Close()
	delete(s.Clients, client.Hash)
}

func (s *PlatypusServer) GetAllPlatypusClients() map[string](*context.TCPClient) {
	return s.Clients
}
