package api

import "time"

// Session is the desktop's view of a connected reverse-shell or termite
// client. It's the union of the JSON-serialised fields from
// internal/core/client.go (TCPClient) and internal/core/termite.go
// (TermiteClient) on the server side.
//
// Encrypted distinguishes the two:
//   - false → plain reverse shell (TCPClient)
//   - true  → encrypted termite (TermiteClient)
type Session struct {
	Hash              string            `json:"hash"`
	Host              string            `json:"host"`
	Port              uint16            `json:"port"`
	Alias             string            `json:"alias"`
	User              string            `json:"user"`
	OS                string            `json:"os"`
	Version           string            `json:"version,omitempty"` // termite only
	NetworkInterfaces map[string]string `json:"network_interfaces"`
	Python2           string            `json:"python2"`
	Python3           string            `json:"python3"`
	Timestamp         time.Time         `json:"timestamp"`
	GroupDispatch     bool              `json:"group_dispatch"`
	Encrypted         bool              `json:"-"` // set by ListSessions based on which map it came from

	// Tag is a UI-friendly synthetic field set by ListSessions to "termite"
	// or "shell" so the frontend doesn't have to look at Encrypted.
	Tag string `json:"-"`
}

// Listener is the desktop's view of a TCPServer entry from /api/server.
type Listener struct {
	Hash           string    `json:"hash"`
	Host           string    `json:"host"`
	Port           uint16    `json:"port"`
	Encrypted      bool      `json:"encrypted"`
	HashFormat     string    `json:"-"`
	GroupDispatch  bool      `json:"group_dispatch"`
	DisableHistory bool      `json:"disable_history"`
	PublicIP       string    `json:"public_ip"`
	ShellPath      string    `json:"shell_path"`
	Timestamp      time.Time `json:"timestamp"`
	Interfaces     []string  `json:"interfaces"`
	NumSessions    int       `json:"-"` // computed by ListListeners from Clients+TermiteClients
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
