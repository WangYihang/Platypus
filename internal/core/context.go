package core

import (
	"strings"

	"github.com/WangYihang/Platypus/internal/app"
	"github.com/WangYihang/Platypus/internal/utils/message"
	"github.com/fatih/color"
)

// WindowSize represents terminal dimensions.
type WindowSize struct {
	Columns int
	Rows    int
}

// Ctx is the global application state. It replaces the former Context struct
// with the App type from the app package.
var Ctx *app.App

// CreateContext initializes the global Ctx and registers gob types.
func CreateContext() {
	// Signal Handler
	Signal()
	// Register gob
	message.RegisterGob()
}

// --- Helper functions that operate on Ctx for backward compatibility ---

func FindTCPClientByHash(hash string) *TCPClient {
	if hash == "" {
		return nil
	}
	for _, s := range Ctx.Servers {
		server := s.(*TCPServer)
		for _, client := range server.GetAllTCPClients() {
			if strings.HasPrefix(client.Hash, strings.ToLower(hash)) {
				return client
			}
		}
	}
	return nil
}

func FindTCPClientByAlias(alias string) *TCPClient {
	if alias == "" {
		return nil
	}
	for _, s := range Ctx.Servers {
		server := s.(*TCPServer)
		for _, client := range server.GetAllTCPClients() {
			if strings.HasPrefix(client.Alias, strings.ToLower(alias)) {
				return client
			}
		}
	}
	return nil
}

func FindTermiteClientByHash(hash string) *TermiteClient {
	if hash == "" {
		return nil
	}
	for _, s := range Ctx.Servers {
		server := s.(*TCPServer)
		for _, client := range server.GetAllTermiteClients() {
			if strings.HasPrefix(client.Hash, strings.ToLower(hash)) {
				return client
			}
		}
	}
	return nil
}

func FindTermiteClientByAlias(alias string) *TermiteClient {
	if alias == "" {
		return nil
	}
	for _, s := range Ctx.Servers {
		server := s.(*TCPServer)
		for _, client := range server.GetAllTermiteClients() {
			if strings.HasPrefix(client.Alias, strings.ToLower(alias)) {
				return client
			}
		}
	}
	return nil
}

func FindServerByHash(hash string) *TCPServer {
	if hash == "" {
		return nil
	}
	for _, s := range Ctx.Servers {
		server := s.(*TCPServer)
		if strings.HasPrefix(server.Hash, strings.ToLower(hash)) {
			return server
		}
	}
	return nil
}

func DeleteTCPClient(c *TCPClient) {
	if Ctx.RLInstance != nil {
		Ctx.RLInstance.SetPrompt(color.CyanString("» "))
	}
	for _, s := range Ctx.Servers {
		server := s.(*TCPServer)
		server.DeleteTCPClient(c)
	}
}

func DeleteTermiteClient(c *TermiteClient) {
	if Ctx.RLInstance != nil {
		Ctx.RLInstance.SetPrompt(color.CyanString("» "))
	}
	for _, s := range Ctx.Servers {
		server := s.(*TCPServer)
		server.DeleteTermiteClient(c)
	}
}

func DeleteServer(s *TCPServer) {
	s.Stop()
	delete(Ctx.Servers, s.Hash)
}

func Shutdown() {
	Ctx.Shutdown()
}

// GetServers returns the typed server map for iteration.
func GetServers() map[string]*TCPServer {
	result := make(map[string]*TCPServer, len(Ctx.Servers))
	for k, v := range Ctx.Servers {
		result[k] = v.(*TCPServer)
	}
	return result
}

func FindServerListeningAddressByRouteKey(routeKey string) string {
	if Ctx.Distributor == nil {
		return ""
	}
	dist := Ctx.Distributor.(*Distributor)
	for k, v := range dist.Route {
		if v == routeKey {
			return k
		}
	}
	return ""
}
