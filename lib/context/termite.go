package context

import (
	"encoding/gob"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
	"gopkg.in/olahol/melody.v1"

	"github.com/WangYihang/Platypus/lib/util/hash"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/message"
	oss "github.com/WangYihang/Platypus/lib/util/os"
	"github.com/WangYihang/Platypus/lib/util/str"
	humanize "github.com/dustin/go-humanize"
	"github.com/jedib0t/go-pretty/table"
)

type ProcessState int

const (
	StartRequested ProcessState = iota
	Started
	TerminatRequested
	Terminated
)

type Process struct {
	Pid           int
	WindowColumns int
	WindowRows    int
	State         ProcessState
	WebSocket     *melody.Session
}

type TermiteClient struct {
	conn              net.Conn
	Hash              string              `json:"hash"`
	Host              string              `json:"host"`
	Port              uint16              `json:"port"`
	Alias             string              `json:"alias"`
	User              string              `json:"user"`
	OS                oss.OperatingSystem `json:"os"`
	NetworkInterfaces map[string]string   `json:"network_interfaces"`
	Python2           string              `json:"python2"`
	Python3           string              `json:"python3"`
	TimeStamp         time.Time           `json:"timestamp"`
	DisableHistory    bool                `json:"disable_hisory"`
	server            *TCPServer
	EncoderLock       *sync.Mutex
	DecoderLock       *sync.Mutex
	AtomLock          *sync.Mutex
	Encoder           *gob.Encoder
	Decoder           *gob.Decoder
	Processes         map[string]*Process
	CurrentProcessKey string
}

func CreateTermiteClient(conn net.Conn, server *TCPServer, disableHistory bool) *TermiteClient {
	host := strings.Split(conn.RemoteAddr().String(), ":")[0]
	port, _ := strconv.Atoi(strings.Split(conn.RemoteAddr().String(), ":")[1])
	return &TermiteClient{
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
		EncoderLock:       new(sync.Mutex),
		DecoderLock:       new(sync.Mutex),
		AtomLock:          new(sync.Mutex),
		Encoder:           gob.NewEncoder(conn),
		Decoder:           gob.NewDecoder(conn),
		Processes:         map[string]*Process{},
		CurrentProcessKey: "",
		DisableHistory:    disableHistory,
	}
}

func (c *TermiteClient) GetHashFormat() string {
	return c.server.hashFormat
}

func (c *TermiteClient) StartSocks5Server() {
	c.EncoderLock.Lock()
	c.Encoder.Encode(message.Message{
		Type: message.DYNAMIC_TUNNEL_CREATE,
		Body: message.BodyDynamicTunnelCreate{},
	})
	c.EncoderLock.Unlock()
}

func (c *TermiteClient) GatherClientInfo(hashFormat string) bool {
	log.Info("Gathering information from termite client...")

	c.AtomLock.Lock()
	defer func() { c.AtomLock.Unlock() }()

	// Send gather info request
	c.EncoderLock.Lock()
	err := c.Encoder.Encode(message.Message{
		Type: message.GET_CLIENT_INFO,
		Body: message.BodyGetClientInfo{},
	})
	c.EncoderLock.Unlock()

	if err != nil {
		// Network
		log.Error("Network error: %s", err)
		return false
	}

	// Read client response
	msg := &message.Message{}
	c.DecoderLock.Lock()
	err = c.Decoder.Decode(msg)
	c.DecoderLock.Unlock()

	if err != nil {
		log.Error("%s", err)
		return false
	}

	if msg.Type == message.CLIENT_INFO {
		clientInfo := msg.Body.(*message.BodyClientInfo)
		c.OS = oss.Parse(clientInfo.OS)
		c.User = clientInfo.User
		c.Python2 = clientInfo.Python2
		c.Python3 = clientInfo.Python3
		c.NetworkInterfaces = clientInfo.NetworkInterfaces
		c.Hash = c.makeHash(hashFormat)
		return true
	} else {
		log.Error("Client sent unexpected message type: %v", msg)
		return false
	}
}

func (c *TermiteClient) NotifyPlatypusWindowSize(columns int, rows int) {
	c.AtomLock.Lock()
	defer func() { c.AtomLock.Unlock() }()

	if _, exists := c.Processes[c.CurrentProcessKey]; exists {
		c.EncoderLock.Lock()
		err := c.Encoder.Encode(message.Message{
			Type: message.WINDOW_SIZE,
			Body: message.BodyWindowSize{
				Key:     c.CurrentProcessKey,
				Columns: columns,
				Rows:    rows,
			},
		})
		c.EncoderLock.Unlock()

		if err != nil {
			// Network
			log.Error("Network error: %s", err)
			Ctx.DeleteTermiteClient(c)
			return
		}
	}
}

func (c *TermiteClient) RequestTerminate(key string) {
	c.AtomLock.Lock()
	defer func() { c.AtomLock.Unlock() }()

	// Find process
	if process, exists := c.Processes[key]; exists {
		c.EncoderLock.Lock()
		err := c.Encoder.Encode(message.Message{
			Type: message.PROCESS_TERMINATE,
			Body: message.BodyTerminateProcess{
				Key: key,
			},
		})
		c.EncoderLock.Unlock()

		process.State = TerminatRequested

		if err != nil {
			// Network
			log.Error("Network error: %s", err)
		}
	} else {
		log.Error("No such process!")
	}
}

func (c *TermiteClient) RequestStartProcess(path string, columns int, rows int, key string) {
	c.AtomLock.Lock()
	defer func() { c.AtomLock.Unlock() }()

	c.EncoderLock.Lock()
	err := c.Encoder.Encode(message.Message{
		Type: message.PROCESS_START,
		Body: message.BodyStartProcess{
			Path:          path,
			WindowColumns: columns,
			WindowRows:    rows,
			Key:           key,
		},
	})
	c.EncoderLock.Unlock()

	if err != nil {
		// Network
		log.Error("Network error: %s", err)
		return
	}
}

func (c *TermiteClient) InteractWith(key string) {
	// Set stdin in raw mode.
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }() // Best effort.

	for {
		process, exists := c.Processes[key]
		if exists && process.State != Terminated {
			buffer := make([]byte, 0x10)
			n, _ := os.Stdin.Read(buffer)
			if n > 0 {
				c.EncoderLock.Lock()
				err = c.Encoder.Encode(message.Message{
					Type: message.STDIO,
					Body: message.BodyStdio{
						Key:  key,
						Data: buffer[0:n],
					},
				})
				c.EncoderLock.Unlock()
				if err != nil {
					// Network
					// TODO: Handle network error
					log.Error("Network error: %s", err)
					break
				}
			}
		} else {
			break
		}
	}
}

func (c *TermiteClient) StartShell() {
	// Get platypus terminal size
	columns, rows, _ := term.GetSize(0)

	key := str.RandomString(0x10)
	c.RequestStartProcess("/bin/bash", columns, rows, key)

	// Create Process Object
	process := Process{
		Pid:           -2,
		WindowColumns: 0,
		WindowRows:    0,
		State:         StartRequested,
		WebSocket:     nil,
	}
	c.Processes[key] = &process

	c.InteractWith(key)
}

func (c *TermiteClient) System(command string) string {
	return "to be done"
}

func (c *TermiteClient) Close() {
	log.Info("Closing client: %s", c.FullDesc())
	for k, ti := range Ctx.PushTunnelInstance {
		if ti.Termite == c && ti.Conn != nil {
			delete(Ctx.PushTunnelInstance, k)
		}
	}
	for k, tc := range Ctx.PushTunnelConfig {
		if tc.Termite == c {
			delete(Ctx.PushTunnelConfig, k)
		}
	}

	for k, ti := range Ctx.PullTunnelInstance {
		if ti.Termite == c && ti.Conn != nil {
			delete(Ctx.PullTunnelInstance, k)
		}
	}
	for k, tc := range Ctx.PullTunnelConfig {
		if tc.Termite == c {
			log.Info("Removing pull tunnel config from %s to %s", (*tc.Server).Addr().String(), tc.Address)
			(*tc.Server).Close()
			delete(Ctx.PullTunnelConfig, k)
		}
	}
	c.conn.Close()
	if Ctx.CurrentTermite == c {
		Ctx.CurrentTermite = nil
	}
}

func (c *TermiteClient) AsTable() {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"Hash", "Network", "OS", "User", "Python", "Time", "Alias"})
	t.AppendRow([]interface{}{
		c.Hash,
		c.conn.RemoteAddr().String(),
		c.OS.String(),
		c.User,
		c.Python2 != "" || c.Python3 != "",
		humanize.Time(c.TimeStamp),
		c.Alias,
	})
	t.Render()
}

func (c *TermiteClient) GetPrompt() string {
	if c.Alias != "" {
		return fmt.Sprintf(
			"[%s] [Encrypted] (%s) %s [%s] » ",
			c.Alias,
			c.OS.String(),
			c.GetConnString(),
			c.GetUsername(),
		)
	}
	return fmt.Sprintf(
		"[Encrypted] (%s) %s [%s] » ",
		c.OS.String(),
		c.GetConnString(),
		c.GetUsername(),
	)
}

func (c *TermiteClient) GetConnString() string {
	return c.conn.RemoteAddr().String()
}

func (c *TermiteClient) GetConn() net.Conn {
	return c.conn
}

func (c *TermiteClient) GetUsername() string {
	var username string
	if c.User == "" {
		username = "unknown"
	} else {
		username = c.User
	}
	return username
}

func (c *TermiteClient) makeHash(hashFormat string) string {
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
				data += value
				data += "\n"
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

func (c *TermiteClient) OnelineDesc() string {
	addr := c.conn.RemoteAddr()
	return fmt.Sprintf("[%s] %s://%s [%s]", c.Hash, addr.Network(), addr.String(), c.OS.String())
}

func (c *TermiteClient) FullDesc() string {
	addr := c.conn.RemoteAddr()
	return fmt.Sprintf("[%s] %s://%s (connected at: %s) [%s]", c.Hash, addr.Network(), addr.String(),
		humanize.Time(c.TimeStamp), c.OS.String())
}
