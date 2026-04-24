package core

import (
	"strings"

	"github.com/WangYihang/Platypus/internal/app"
)

// WindowSize represents terminal dimensions for the websocket-driven
// remote shell. Browsers serialise this as JSON on a TTY-resize message.
type WindowSize struct {
	Columns int
	Rows    int
}

// Ctx is the global application state.
var Ctx *app.App

// allAgentClients returns a snapshot of every live AgentClient the
// process knows about. Iteration is safe — callers get a shallow copy
// of the agent pointers and don't hold the AgentService lock.
func allAgentClients() map[string]*AgentClient {
	if agentSvc == nil {
		return nil
	}
	return agentSvc.Snapshot()
}

// AllAgents is the exported version of allAgentClients. It exists so
// api/* handlers that used to iterate Ctx.Servers can query the
// registered AgentService without reaching into core internals.
func AllAgents() map[string]*AgentClient { return allAgentClients() }

func FindAgentClientByHash(hash string) *AgentClient {
	if hash == "" {
		return nil
	}
	for _, client := range allAgentClients() {
		if strings.HasPrefix(client.Hash, strings.ToLower(hash)) {
			return client
		}
	}
	return nil
}

func FindAgentClientByAlias(alias string) *AgentClient {
	if alias == "" {
		return nil
	}
	for _, client := range allAgentClients() {
		if strings.HasPrefix(client.Alias, strings.ToLower(alias)) {
			return client
		}
	}
	return nil
}

// DeleteAgentClient removes the client from the service and fires the
// disconnect bookkeeping (activity record, session row stamp). Callers
// in the message-dispatch loop invoke this when Recv returns an error.
func DeleteAgentClient(c *AgentClient) {
	if agentSvc == nil || c == nil {
		return
	}
	agentSvc.removeClient(c)
}
