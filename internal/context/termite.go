package context

import (
	"encoding/gob"
	"fmt"
	"github.com/WangYihang/Platypus/internal/context/Models"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/mod/semver"
	"golang.org/x/term"
	"gopkg.in/olahol/melody.v1"

	"github.com/WangYihang/Platypus/internal/util/hash"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/WangYihang/Platypus/internal/util/message"
	oss "github.com/WangYihang/Platypus/internal/util/os"
	"github.com/WangYihang/Platypus/internal/util/str"
	"github.com/WangYihang/Platypus/internal/util/update"
	humanize "github.com/dustin/go-humanize"
	"github.com/jedib0t/go-pretty/table"
)

type processState int

const (
	startRequested processState = iota
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

type TermiteClient struct {
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
	server            *TCPServer          `json:"-"`
	encoderLock       *sync.Mutex         `json:"-"`
	decoderLock       *sync.Mutex         `json:"-"`
	atomLock          *sync.Mutex         `json:"-"`
	encoder           *gob.Encoder        `json:"-"`
	decoder           *gob.Decoder        `json:"-"`
	processes         map[string]*Process `json:"-"`
	currentProcessKey string              `json:"-"`
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
		encoderLock:       new(sync.Mutex),
		decoderLock:       new(sync.Mutex),
		atomLock:          new(sync.Mutex),
		encoder:           gob.NewEncoder(conn),
		decoder:           gob.NewDecoder(conn),
		processes:         map[string]*Process{},
		currentProcessKey: "",
		DisableHistory:    disableHistory,
		GroupDispatch:     false,
	}
}

func (c *TermiteClient) LockAtom() {
	c.atomLock.Lock()
}

func (c *TermiteClient) UnlockAtom() {
	c.atomLock.Unlock()
}

func (c *TermiteClient) LockEncoder() {
	c.encoderLock.Lock()
}

func (c *TermiteClient) UnlockEncoder() {
	c.encoderLock.Unlock()
}

func (c *TermiteClient) LockDecoder() {
	c.decoderLock.Lock()
}

func (c *TermiteClient) UnlockDecoder() {
	c.decoderLock.Unlock()
}

func (c *TermiteClient) GetHashFormat() string {
	return c.server.hashFormat
}

func (c *TermiteClient) GetShellPath() string {
	return c.server.ShellPath
}

func (c *TermiteClient) StartSocks5Server() {
	c.Send(message.Message{
		Type: message.DYNAMIC_TUNNEL_CREATE,
		Body: message.BodyDynamicTunnelCreate{},
	})
}

func (c *TermiteClient) GatherClientInfo(hashFormat string) bool {
	log.Info("Gathering information from termite client...")

	c.LockAtom()
	defer c.UnlockAtom()

	// Send gather info request
	err := c.Send(message.Message{
		Type: message.GET_CLIENT_INFO,
		Body: message.BodyGetClientInfo{},
	})

	if err != nil {
		// Network
		log.Error("Network error: %s", err)
		return false
	}

	// Read client response
	msg := message.Message{}
	c.Recv(&msg)

	if err != nil {
		log.Error("%s", err)
		return false
	}

	if msg.Type == message.CLIENT_INFO {
		if msg.Body != nil {
			clientInfo := msg.Body.(*message.BodyClientInfo)
			c.Version = clientInfo.Version
			log.Info("Client version: v%s", c.Version)
			c.OS = oss.Parse(clientInfo.OS)
			c.User = clientInfo.User
			c.Python2 = clientInfo.Python2
			c.Python3 = clientInfo.Python3
			c.NetworkInterfaces = clientInfo.NetworkInterfaces
			c.Hash = c.makeHash(hashFormat)
			if semver.Compare(fmt.Sprintf("v%s", update.Version), fmt.Sprintf("v%s", c.Version)) > 0 {
				// Termite needs up to date
				c.Send(message.Message{
					Type: message.UPDATE,
					Body: message.BodyUpdate{
						DistributorURL: Ctx.Distributor.Url,
						Version:        update.Version,
					},
				})
				return false
			}
			Models.CreateAccess(&Models.Access{
				Host:      c.Host,
				Port:      c.Port,
				Hash:      c.Hash,
				TimeStamp: c.TimeStamp,
				User:      c.User,
				OS:        c.OS,
			})

			return true
		} else {
			log.Error("Client sent empty client info body: %v", msg)
			return false
		}
	} else {
		log.Error("Client sent unexpected message type: %v", msg)
		return false
	}
}

func (c *TermiteClient) Send(message message.Message) error {
	c.LockEncoder()
	err := c.encoder.Encode(message)
	c.UnlockEncoder()
	return err
}

func (c *TermiteClient) Recv(msg *message.Message) error {
	c.LockDecoder()
	err := c.decoder.Decode(msg)
	c.UnlockDecoder()
	return err
}

func (c *TermiteClient) NotifyPlatypusWindowSize(columns int, rows int) {
	c.LockAtom()
	defer c.UnlockAtom()

	if _, exists := c.processes[c.currentProcessKey]; exists {
		err := c.Send(message.Message{
			Type: message.WINDOW_SIZE,
			Body: message.BodyWindowSize{
				Key:     c.currentProcessKey,
				Columns: columns,
				Rows:    rows,
			},
		})

		if err != nil {
			// Network
			log.Error("Network error: %s", err)
			Ctx.DeleteTermiteClient(c)
			return
		}
	}
}

func (c *TermiteClient) RequestTerminate(key string) {
	c.LockAtom()
	defer c.UnlockAtom()

	// Find process
	if process, exists := c.processes[key]; exists {
		err := c.Send(message.Message{
			Type: message.PROCESS_TERMINATE,
			Body: message.BodyTerminateProcess{
				Key: key,
			},
		})

		process.State = terminatRequested

		if err != nil {
			// Network
			log.Error("Network error: %s", err)
		}
	} else {
		log.Error("No such process!")
	}
}

func (c *TermiteClient) RequestStartProcess(path string, columns int, rows int, key string) {
	c.LockAtom()
	defer c.UnlockAtom()

	err := c.Send(message.Message{
		Type: message.PROCESS_START,
		Body: message.BodyStartProcess{
			Path:          path,
			WindowColumns: columns,
			WindowRows:    rows,
			Key:           key,
		},
	})

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
		process, exists := c.processes[key]
		if exists && process.State != terminated {
			buffer := make([]byte, 0x10)
			n, _ := os.Stdin.Read(buffer)
			if n > 0 {
				err = c.Send(message.Message{
					Type: message.STDIO,
					Body: message.BodyStdio{
						Key:  key,
						Data: buffer[0:n],
					},
				})
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
	c.RequestStartProcess(c.GetShellPath(), columns, rows, key)

	// Create Process Object
	process := Process{
		Pid:           -2,
		WindowColumns: 0,
		WindowRows:    0,
		State:         startRequested,
		WebSocket:     nil,
	}
	c.processes[key] = &process

	c.InteractWith(key)
}

func (c *TermiteClient) System(command string) string {
	token := uuid.New().String()
	Ctx.MessageQueue[token] = make(chan message.Message)
	err := c.Send(message.Message{
		Type: message.CALL_SYSTEM,
		Body: message.BodyCallSystem{
			Command: command,
			Token:   token,
		},
	})
	if err != nil {
		return err.Error()
	}
	msg := <-Ctx.MessageQueue[token]

	if msg.Type == message.CALL_SYSTEM_RESULT {
		result := msg.Body.(*message.BodyCallSystemResult).Result
		return string(result)
	} else {
		return ""
	}
}

func (c *TermiteClient) FileSize(path string) (int64, error) {
	token := uuid.New().String()
	Ctx.MessageQueue[token] = make(chan message.Message)
	c.Send(message.Message{
		Type: message.FILE_SIZE,
		Body: message.BodyFileSize{
			Path:  path,
			Token: token,
		},
	})
	msg := <-Ctx.MessageQueue[token]

	if msg.Type == message.FILE_SIZE_RESULT {
		n := msg.Body.(*message.BodyFileSizeResult).N
		if n < 0 {
			return n, fmt.Errorf("get file size failed")
		} else {
			return n, nil
		}
	} else {
		return -1, fmt.Errorf("invalid message type: %v", msg.Type)
	}
}

func (c *TermiteClient) ReadFileEx(path string, start int64, size int64) ([]byte, error) {
	token := uuid.New().String()
	Ctx.MessageQueue[token] = make(chan message.Message)

	c.Send(message.Message{
		Type: message.READ_FILE_EX,
		Body: message.BodyReadFileEx{
			Path:  path,
			Start: start,
			Size:  size,
			Token: token,
		},
	})

	msg := <-Ctx.MessageQueue[token]
	if msg.Type == message.READ_FILE_EX_RESULT {
		result := msg.Body.(*message.BodyReadFileExResult).Result
		return result, nil
	} else {
		return nil, fmt.Errorf("invalid message type: %v", msg.Type)
	}
}

func (c *TermiteClient) ReadFile(path string) ([]byte, error) {
	token := uuid.New().String()
	Ctx.MessageQueue[token] = make(chan message.Message)
	c.Send(message.Message{
		Type: message.READ_FILE,
		Body: message.BodyReadFile{
			Path:  path,
			Token: token,
		},
	})
	msg := <-Ctx.MessageQueue[token]

	if msg.Type == message.READ_FILE_RESULT {
		result := msg.Body.(*message.BodyReadFileResult).Result
		return result, nil
	} else {
		return nil, fmt.Errorf("invalid message type: %v", msg.Type)
	}
}

func (c *TermiteClient) WriteFile(path string, content []byte) (int, error) {
	token := uuid.New().String()
	Ctx.MessageQueue[token] = make(chan message.Message)
	c.Send(message.Message{
		Type: message.WRITE_FILE,
		Body: message.BodyWriteFile{
			Path:    path,
			Content: content,
			Token:   token,
		},
	})
	msg := <-Ctx.MessageQueue[token]

	if msg.Type == message.WRITE_FILE_RESULT {
		n := msg.Body.(*message.BodyWriteFileResult).N
		return n, nil
	} else {
		return -1, fmt.Errorf("invalid message type: %v", msg.Type)
	}
}

func (c *TermiteClient) WriteFileEx(path string, content []byte) (int, error) {
	token := uuid.New().String()
	Ctx.MessageQueue[token] = make(chan message.Message)
	c.Send(message.Message{
		Type: message.WRITE_FILE_EX,
		Body: message.BodyWriteFileEx{
			Path:    path,
			Content: content,
			Token:   token,
		},
	})
	msg := <-Ctx.MessageQueue[token]

	if msg.Type == message.WRITE_FILE_EX_RESULT {
		n := msg.Body.(*message.BodyWriteFileExResult).N
		return n, nil
	} else {
		return -1, fmt.Errorf("invalid message type: %v", msg.Type)
	}
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
	t.AppendHeader(table.Row{"Hash", "Network", "OS", "User", "Python", "Time", "Alias", "GroupDispatch"})
	t.AppendRow([]interface{}{
		c.Hash,
		c.conn.RemoteAddr().String(),
		c.OS.String(),
		c.User,
		c.Python2 != "" || c.Python3 != "",
		humanize.Time(c.TimeStamp),
		c.Alias,
		c.GroupDispatch,
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
	return fmt.Sprintf("[%s] %s://%s (connected at: %s) [%s] [%t]", c.Hash, addr.Network(), addr.String(),
		humanize.Time(c.TimeStamp), c.OS.String(), c.GroupDispatch)
}

func (c *TermiteClient) AddProcess(key string, process *Process) {
	c.processes[key] = process
}
