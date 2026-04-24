package api

import "time"

// Session is the desktop's view of a connected agent. It mirrors the
// JSON-serialised fields of internal/core/agent.go (AgentClient) on
// the server side.
type Session struct {
	Hash              string            `json:"hash"`
	Host              string            `json:"host"`
	Port              uint16            `json:"port"`
	Alias             string            `json:"alias"`
	User              string            `json:"user"`
	OS                string            `json:"os"`
	Version           string            `json:"version,omitempty"`
	NetworkInterfaces map[string]string `json:"network_interfaces"`
	Python2           string            `json:"python2"`
	Python3           string            `json:"python3"`
	Timestamp         time.Time         `json:"timestamp"`
	GroupDispatch     bool              `json:"group_dispatch"`
}

// TunnelInfo mirrors what the server's GET /api/v1/sessions/:id/tunnels
// returns: each entry has a type (pull|push|socks5) and an address.
type TunnelInfo struct {
	Type    string `json:"type"`
	Address string `json:"address"`
}

// DispatchResult is one row of POST /api/v1/sessions/dispatch's response.
type DispatchResult struct {
	SessionHash string `json:"session_hash"`
	Output      string `json:"output"`
	Error       string `json:"error,omitempty"`
}
