package core

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/mod/semver"
	"gopkg.in/olahol/melody.v1"

	humanize "github.com/dustin/go-humanize"

	"github.com/WangYihang/Platypus/internal/activity"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/protocol"
	"github.com/WangYihang/Platypus/internal/session"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/utils/hash"
	oss "github.com/WangYihang/Platypus/internal/utils/os"
	"github.com/WangYihang/Platypus/internal/utils/update"
	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// Compile-time check: AgentClient implements session.Session
var _ session.Session = (*AgentClient)(nil)

type processState int

const (
	StartRequested processState = iota
	started
	terminatRequested
	terminated
)

type Process struct {
	Pid           int
	WindowColumns int
	WindowRows    int
	State         processState
	WebSocket     *melody.Session
}

type AgentClient struct {
	conn              net.Conn            `json:"-"`
	Hash              string              `json:"hash"`
	Host              string              `json:"host"`
	Port              uint16              `json:"port"`
	Alias             string              `json:"alias"`
	User              string              `json:"user"`
	OS                oss.OperatingSystem `json:"os"`
	Version           string              `json:"version"`
	NetworkInterfaces map[string]string   `json:"network_interfaces"`
	Python2           string              `json:"python2"`
	Python3           string              `json:"python3"`
	TimeStamp         time.Time           `json:"timestamp"`
	DisableHistory    bool                `json:"disable_hisory"`
	GroupDispatch     bool                `json:"group_dispatch"`
	// MachineID is the agent-reported stable id (see internal/agent/machine_id.go).
	// Empty when the agent couldn't read a platform id; the server falls back
	// to a hash of hostname + sorted MACs for host aggregation in that case.
	MachineID string `json:"machine_id"`
	Hostname  string `json:"hostname"`
	// HostID + ProjectID are stamped by UpsertHostForAgent on successful
	// handshake, pointing at the storage.Host row this session belongs to.
	HostID            string               `json:"host_id"`
	ProjectID         string               `json:"project_id"`
	server            *TCPServer           `json:"-"`
	codec             *protocol.ProtoCodec `json:"-"`
	atomLock          *sync.Mutex          `json:"-"`
	processes         map[string]*Process  `json:"-"`
	currentProcessKey string               `json:"-"`
}

func CreateAgentClient(conn net.Conn, server *TCPServer, disableHistory bool) *AgentClient {
	host := strings.Split(conn.RemoteAddr().String(), ":")[0]
	port, _ := strconv.Atoi(strings.Split(conn.RemoteAddr().String(), ":")[1])
	return &AgentClient{
		conn:              conn,
		Hash:              "",
		Host:              host,
		Port:              uint16(port),
		Alias:             "",
		User:              "",
		OS:                oss.Unknown,
		NetworkInterfaces: map[string]string{},
		Python2:           "",
		Python3:           "",
		TimeStamp:         time.Now(),
		server:            server,
		codec:             protocol.NewProtoCodec(conn),
		atomLock:          new(sync.Mutex),
		processes:         map[string]*Process{},
		currentProcessKey: "",
		DisableHistory:    disableHistory,
		GroupDispatch:     false,
	}
}

func (c *AgentClient) GetHash() string            { return c.Hash }
func (c *AgentClient) GetAlias() string           { return c.Alias }
func (c *AgentClient) SetAlias(alias string)      { c.Alias = alias }
func (c *AgentClient) GetHost() string            { return c.Host }
func (c *AgentClient) GetPort() uint16            { return c.Port }
func (c *AgentClient) GetOS() oss.OperatingSystem { return c.OS }
func (c *AgentClient) GetTimeStamp() time.Time    { return c.TimeStamp }
func (c *AgentClient) GetGroupDispatch() bool     { return c.GroupDispatch }
func (c *AgentClient) SetGroupDispatch(v bool)    { c.GroupDispatch = v }

func (c *AgentClient) LockAtom()   { c.atomLock.Lock() }
func (c *AgentClient) UnlockAtom() { c.atomLock.Unlock() }

func (c *AgentClient) GetHashFormat() string { return c.server.hashFormat }
func (c *AgentClient) GetShellPath() string  { return c.server.ShellPath }

// Send sends a protobuf envelope to the agent.
func (c *AgentClient) Send(env *agentpb.Envelope) error {
	if env.Version == 0 {
		env.Version = 1
	}
	if env.Timestamp == 0 {
		env.Timestamp = time.Now().UnixNano()
	}
	return c.codec.Send(env)
}

// Recv receives a protobuf envelope from the agent.
func (c *AgentClient) Recv() (*agentpb.Envelope, error) {
	return c.codec.Recv()
}

func (c *AgentClient) StartSocks5Server() {
	c.Send(&agentpb.Envelope{
		Payload: &agentpb.Envelope_Socks5CreateRequest{
			Socks5CreateRequest: &agentpb.Socks5CreateRequest{},
		},
	})
}

func (c *AgentClient) GatherClientInfo(hashFormat string) bool {
	log.Info("Gathering information from agent...")

	c.LockAtom()
	defer c.UnlockAtom()

	err := c.Send(&agentpb.Envelope{
		Payload: &agentpb.Envelope_GetClientInfoRequest{
			GetClientInfoRequest: &agentpb.GetClientInfoRequest{},
		},
	})
	if err != nil {
		log.Error("Network error: %s", err)
		return false
	}

	env, err := c.Recv()
	if err != nil {
		log.Error("Recv error: %s", err)
		return false
	}

	info := env.GetClientInfoResponse()
	if info == nil {
		log.Error("Client sent unexpected message: %v", env)
		return false
	}

	c.Version = info.Version
	log.Info("Client version: v%s", c.Version)
	c.OS = oss.Parse(info.Os)
	c.User = info.User
	c.Python2 = ""
	c.Python3 = ""
	for _, lang := range info.AvailableLanguages {
		if lang == "python2" {
			c.Python2 = "python2"
		}
		if lang == "python3" {
			c.Python3 = "python3"
		}
	}
	c.NetworkInterfaces = info.NetworkInterfaces
	c.MachineID = info.MachineId
	c.Hostname = info.Hostname
	c.Hash = c.makeHash(hashFormat)

	if semver.Compare(fmt.Sprintf("v%s", update.Version), fmt.Sprintf("v%s", c.Version)) > 0 {
		dist := Ctx.Distributor.(*Distributor)
		c.Send(&agentpb.Envelope{
			Payload: &agentpb.Envelope_UpdateRequest{
				UpdateRequest: &agentpb.UpdateRequest{
					BaseUrl: dist.Url,
					Version: update.Version,
					Channel: dist.Channel,
				},
			},
		})
		return false
	}
	return true
}

func (c *AgentClient) RequestTerminate(key string) {
	c.LockAtom()
	defer c.UnlockAtom()

	if process, exists := c.processes[key]; exists {
		err := c.Send(&agentpb.Envelope{
			Payload: &agentpb.Envelope_ProcessTerminateRequest{
				ProcessTerminateRequest: &agentpb.ProcessTerminateRequest{Key: key},
			},
		})
		process.State = terminatRequested
		if err != nil {
			log.Error("Network error: %s", err)
		}
	} else {
		log.Error("No such process!")
	}
}

func (c *AgentClient) RequestStartProcess(path string, columns int, rows int, key string) {
	c.LockAtom()
	defer c.UnlockAtom()

	err := c.Send(&agentpb.Envelope{
		Payload: &agentpb.Envelope_ProcessStartRequest{
			ProcessStartRequest: &agentpb.ProcessStartRequest{
				Path:          path,
				WindowColumns: int32(columns),
				WindowRows:    int32(rows),
				Key:           key,
			},
		},
	})
	if err != nil {
		log.Error("Network error: %s", err)
	}
}

// Execute runs a command on the remote and returns its output.
func (c *AgentClient) Execute(command string) (string, error) {
	return c.System(command), nil
}

// System sends an exec request and blocks for the response via MessageQueue.
func (c *AgentClient) System(command string) string {
	token := uuid.New().String()
	ch := make(chan interface{}, 1)
	Ctx.MessageQueueMu.Lock()
	Ctx.EnvelopeQueue[token] = ch
	Ctx.MessageQueueMu.Unlock()

	err := c.Send(&agentpb.Envelope{
		RequestId: token,
		Payload: &agentpb.Envelope_ExecRequest{
			ExecRequest: &agentpb.ExecRequest{Command: command},
		},
	})
	if err != nil {
		return err.Error()
	}

	env := (<-ch).(*agentpb.Envelope)
	if resp := env.GetExecResponse(); resp != nil {
		return string(resp.Output)
	}
	return ""
}

func (c *AgentClient) FileSize(path string) (int64, error) {
	token := uuid.New().String()
	ch := make(chan interface{}, 1)
	Ctx.MessageQueueMu.Lock()
	Ctx.EnvelopeQueue[token] = ch
	Ctx.MessageQueueMu.Unlock()

	c.Send(&agentpb.Envelope{
		RequestId: token,
		Payload: &agentpb.Envelope_FileSizeRequest{
			FileSizeRequest: &agentpb.FileSizeRequest{Path: path},
		},
	})

	env := (<-ch).(*agentpb.Envelope)
	if resp := env.GetFileSizeResponse(); resp != nil {
		if resp.Error != "" {
			return -1, fmt.Errorf("%s", resp.Error)
		}
		return resp.Size, nil
	}
	return -1, fmt.Errorf("invalid response")
}

func (c *AgentClient) ReadFileEx(path string, start int64, size int64) ([]byte, error) {
	token := uuid.New().String()
	ch := make(chan interface{}, 1)
	Ctx.MessageQueueMu.Lock()
	Ctx.EnvelopeQueue[token] = ch
	Ctx.MessageQueueMu.Unlock()

	c.Send(&agentpb.Envelope{
		RequestId: token,
		Payload: &agentpb.Envelope_ReadFileRequest{
			ReadFileRequest: &agentpb.ReadFileRequest{Path: path, Offset: start, Size: size},
		},
	})

	env := (<-ch).(*agentpb.Envelope)
	if resp := env.GetReadFileResponse(); resp != nil {
		if resp.Error != "" {
			return nil, fmt.Errorf("%s", resp.Error)
		}
		return resp.Data, nil
	}
	return nil, fmt.Errorf("invalid response")
}

func (c *AgentClient) ReadFile(path string) ([]byte, error) {
	return c.ReadFileEx(path, 0, 0) // size=0 means read all
}

func (c *AgentClient) WriteFile(path string, content []byte) (int, error) {
	return c.writeFileInternal(path, content, false)
}

func (c *AgentClient) WriteFileEx(path string, content []byte) (int, error) {
	return c.writeFileInternal(path, content, true)
}

func (c *AgentClient) writeFileInternal(path string, content []byte, appendMode bool) (int, error) {
	token := uuid.New().String()
	ch := make(chan interface{}, 1)
	Ctx.MessageQueueMu.Lock()
	Ctx.EnvelopeQueue[token] = ch
	Ctx.MessageQueueMu.Unlock()

	c.Send(&agentpb.Envelope{
		RequestId: token,
		Payload: &agentpb.Envelope_WriteFileRequest{
			WriteFileRequest: &agentpb.WriteFileRequest{Path: path, Data: content, Append: appendMode},
		},
	})

	env := (<-ch).(*agentpb.Envelope)
	if resp := env.GetWriteFileResponse(); resp != nil {
		if resp.Error != "" {
			return -1, fmt.Errorf("%s", resp.Error)
		}
		return int(resp.BytesWritten), nil
	}
	return -1, fmt.Errorf("invalid response")
}

// recordSessionOpen writes one session.open row when an agent has
// finished the handshake and is registered into the server's active
// client list. The ProjectID is left nil here — the agent session DB
// row (if any) carries the project, and the Activities UI can resolve
// project_id at query time when needed.
func recordSessionOpen(c *AgentClient) {
	if c == nil {
		return
	}
	activity.Record(activity.Input{
		ActorType:   storage.ActorTypeAgent,
		Category:    storage.CategorySession,
		Action:      "session.open",
		TargetType:  "session",
		TargetID:    c.Hash,
		TargetLabel: c.OnelineDesc(),
		SessionID:   c.Hash,
		ActorIP:     c.GetConnString(),
		Meta: map[string]any{
			"host":    c.Host,
			"os":      c.OS.String(),
			"user":    c.User,
			"alias":   c.Alias,
			"version": c.Version,
		},
	})
}

func (c *AgentClient) Close() {
	log.Info("Closing client: %s", c.FullDesc())
	activity.Record(activity.Input{
		ActorType:   storage.ActorTypeAgent,
		Category:    storage.CategorySession,
		Action:      "session.close",
		TargetType:  "session",
		TargetID:    c.Hash,
		TargetLabel: c.OnelineDesc(),
		SessionID:   c.Hash,
		ActorIP:     c.GetConnString(),
		Meta: map[string]any{
			"host":       c.Host,
			"os":         c.OS.String(),
			"user":       c.User,
			"alias":      c.Alias,
			"duration_s": time.Since(c.TimeStamp).Seconds(),
		},
	})
	for k, ti := range Ctx.PushTunnelInstance {
		if ti.Agent == c && ti.Conn != nil {
			delete(Ctx.PushTunnelInstance, k)
		}
	}
	for k, tc := range Ctx.PushTunnelConfig {
		if tc.Agent == c {
			delete(Ctx.PushTunnelConfig, k)
		}
	}
	for k, ti := range Ctx.PullTunnelInstance {
		if ti.Agent == c && ti.Conn != nil {
			delete(Ctx.PullTunnelInstance, k)
		}
	}
	for k, tc := range Ctx.PullTunnelConfig {
		if tc.Agent == c {
			log.Info("Removing pull tunnel config from %s to %s", (*tc.Server).Addr().String(), tc.Address)
			(*tc.Server).Close()
			delete(Ctx.PullTunnelConfig, k)
		}
	}
	c.conn.Close()
	if Ctx.CurrentAgent == c {
		Ctx.CurrentAgent = nil
	}
}

func (c *AgentClient) GetConnString() string { return c.conn.RemoteAddr().String() }
func (c *AgentClient) GetConn() net.Conn     { return c.conn }

func (c *AgentClient) GetUsername() string {
	if c.User == "" {
		return "unknown"
	}
	return c.User
}

func (c *AgentClient) makeHash(hashFormat string) string {
	data := ""
	if c.OS == oss.Linux {
		components := strings.Split(hashFormat, " ")
		mapping := map[string]string{
			"%i": strings.Split(c.conn.RemoteAddr().String(), ":")[0],
			"%u": c.User,
			"%o": c.OS.String(),
			"%m": fmt.Sprintf("%s", c.NetworkInterfaces),
			"%t": c.TimeStamp.String(),
		}
		for _, component := range components {
			if value, exists := mapping[component]; exists {
				data += value + "\n"
			} else {
				data += component
			}
		}
	} else {
		data = c.conn.RemoteAddr().String()
	}
	log.Debug("Hashing: %s", data)
	return hash.MD5(data)
}

func (c *AgentClient) OnelineDesc() string {
	addr := c.conn.RemoteAddr()
	return fmt.Sprintf("[%s] %s://%s [%s]", c.Hash, addr.Network(), addr.String(), c.OS.String())
}

func (c *AgentClient) FullDesc() string {
	addr := c.conn.RemoteAddr()
	return fmt.Sprintf("[%s] %s://%s (connected at: %s) [%s] [%t]", c.Hash, addr.Network(), addr.String(),
		humanize.Time(c.TimeStamp), c.OS.String(), c.GroupDispatch)
}

func (c *AgentClient) AddProcess(key string, process *Process) {
	c.processes[key] = process
}
