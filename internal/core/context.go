package core

import (
	"strings"

	"github.com/WangYihang/Platypus/internal/app"
)

// WindowSize represents terminal dimensions.
type WindowSize struct {
	Columns int
	Rows    int
}

// Ctx is the global application state. It replaces the former Context struct
// with the App type from the app package.
var Ctx *app.App

// CreateContext initializes signal handlers.
func CreateContext() {
	Signal()
}

// --- Helper functions that operate on Ctx for backward compatibility ---

func FindAgentClientByHash(hash string) *AgentClient {
	if hash == "" {
		return nil
	}
	for _, s := range Ctx.Servers {
		server := s.(*TCPServer)
		for _, client := range server.GetAllAgentClients() {
			if strings.HasPrefix(client.Hash, strings.ToLower(hash)) {
				return client
			}
		}
	}
	return nil
}

func FindAgentClientByAlias(alias string) *AgentClient {
	if alias == "" {
		return nil
	}
	for _, s := range Ctx.Servers {
		server := s.(*TCPServer)
		for _, client := range server.GetAllAgentClients() {
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

func DeleteAgentClient(c *AgentClient) {
	for _, s := range Ctx.Servers {
		server := s.(*TCPServer)
		server.DeleteAgentClient(c)
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

