package core

import (
	"context"
	"encoding/json"
	"net"
	"sync"

	"github.com/WangYihang/Platypus/internal/log"
)

// AgentServiceConfig bundles the operator-supplied knobs that define
// how the unified-ingress agent handler treats incoming agent
// connections. All fields are optional; zero values fall back to the
// same defaults CreateTCPServer uses.
type AgentServiceConfig struct {
	HashFormat     string
	ShellPath      string
	IngressAddr    string
	ProjectID      string
	DisableHistory bool
}

// AgentService is the ingress-side replacement for TCPServer. The
// unified dispatcher strips TLS + ALPN and hands the resulting
// net.Conn to Handle; AgentService owns the post-accept pipeline
// (enrollment, info gathering, host/session persistence, client
// registration). It is not tied to a listening socket so a single
// instance serves every agent connection for the process.
type AgentService struct {
	cfg AgentServiceConfig

	mu      sync.RWMutex
	clients map[string]*AgentClient
}

// NewAgentService constructs a service pre-populated with sensible
// defaults. Empty HashFormat / ShellPath are resolved in
// CreateAgentClient so new fields added later inherit the same
// behaviour.
func NewAgentService(cfg AgentServiceConfig) *AgentService {
	return &AgentService{
		cfg:     cfg,
		clients: map[string]*AgentClient{},
	}
}

// Handle is the dispatcher callback. The TLS handshake and ALPN
// selection have already happened; conn is a live
// application-layer connection speaking the agent protobuf.
func (s *AgentService) Handle(conn net.Conn) {
	if s == nil {
		log.Warn("agent service: Handle called before initialisation; dropping %s",
			conn.RemoteAddr())
		_ = conn.Close()
		return
	}
	client := CreateAgentClient(conn, AgentClientConfig{
		HashFormat:     s.cfg.HashFormat,
		ShellPath:      s.cfg.ShellPath,
		IngressAddr:    s.cfg.IngressAddr,
		ProjectID:      s.cfg.ProjectID,
		DisableHistory: s.cfg.DisableHistory,
	}, nil)
	handleAgentConnection(client, nil)
}

// IngressAddr is exposed for callers (install-script templating,
// /api/v1/info) that need to render the host:port an agent should
// dial.
func (s *AgentService) IngressAddr() string {
	if s == nil {
		return ""
	}
	return s.cfg.IngressAddr
}

// ProjectID is the default project id newly-connected agents land
// in before the enrollment outcome overrides it.
func (s *AgentService) ProjectID() string {
	if s == nil {
		return ""
	}
	return s.cfg.ProjectID
}

// Snapshot returns a point-in-time copy of the current client map.
// The shared map is not exposed so callers cannot mutate the
// service's internal state while iterating.
func (s *AgentService) Snapshot() map[string]*AgentClient {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]*AgentClient, len(s.clients))
	for k, v := range s.clients {
		out[k] = v
	}
	return out
}

// addClient registers the finished handshake. Called from
// handleAgentConnection. Duplicate hashes raise the same notice
// TCPServer.AddAgentClient raises (the websocket frame +
// AgentMessageDispatcher goroutine start happens inline).
func (s *AgentService) addClient(client *AgentClient) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if _, exists := s.clients[client.Hash]; exists {
		s.mu.Unlock()
		log.Error("Duplicated income connection detected!")
		s.notifyDuplicate(client)
		client.Close()
		return
	}
	s.clients[client.Hash] = client
	s.mu.Unlock()

	log.Success("Encrypted fire in the hole: %s", client.OnelineDesc())
	s.notifyOnline(client)
	go AgentMessageDispatcher(client)
}

// removeClient drops a client from the service and stamps the
// session row disconnected. Mirrors TCPServer.DeleteAgentClient.
func (s *AgentService) removeClient(client *AgentClient) {
	if s == nil || client == nil {
		return
	}
	s.mu.Lock()
	delete(s.clients, client.Hash)
	s.mu.Unlock()

	DropSysInfo(client.Hash)
	MarkSessionDisconnected(context.Background(), client)
	client.Close()
}

func (s *AgentService) notifyOnline(client *AgentClient) {
	if Ctx == nil || Ctx.NotifyWebSocket == nil {
		return
	}
	msg, err := json.Marshal(WebSocketMessage{
		Type: CLIENT_CONNECTED,
		Data: map[string]any{
			"Client":     client,
			"ServerHash": "", // retained for wire-format stability
		},
	})
	if err != nil {
		return
	}
	_ = Ctx.NotifyWebSocket.Broadcast(msg)
}

func (s *AgentService) notifyDuplicate(client *AgentClient) {
	if Ctx == nil || Ctx.NotifyWebSocket == nil {
		return
	}
	msg, err := json.Marshal(WebSocketMessage{
		Type: CLIENT_DUPLICATED,
		Data: map[string]any{
			"Client":     client,
			"ServerHash": "",
		},
	})
	if err != nil {
		return
	}
	_ = Ctx.NotifyWebSocket.Broadcast(msg)
}

// agentSvc is the process-wide AgentService set by SetAgentService.
// Its value is read without locking because the only writer is
// startup and the only readers are goroutines launched after
// startup.
var agentSvc *AgentService

// SetAgentService registers the process-wide AgentService.
// FindAgentClientByHash / FindAgentClientByAlias consult it alongside
// the legacy Ctx.Servers map so both paths coexist during the
// unified-ingress transition.
func SetAgentService(s *AgentService) { agentSvc = s }

// Agents returns the process-wide AgentService. Nil when the server
// is still running the pre-unified-ingress code path.
func Agents() *AgentService { return agentSvc }
