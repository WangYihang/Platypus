package context

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/WangYihang/Platypus/lib/util/hash"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/str"
	humanize "github.com/dustin/go-humanize"
	"github.com/jedib0t/go-pretty/table"
)

type OperatingSystem int

const (
	Unknown OperatingSystem = iota
	Linux
	Windows
	SunOS
	MacOS
	FreeBSD
)

func (os OperatingSystem) String() string {
	return [...]string{"Unknown", "🐧", "❖", "SunOS", "🍎", "FreeBSD"}[os]
}

type TCPClient struct {
	TimeStamp   time.Time
	Conn        net.Conn
	Interactive bool
	Group       bool
	Hash        string
	User        string
	OS          OperatingSystem
	Python2     string
	Python3     string
	ReadLock    *sync.Mutex
	WriteLock   *sync.Mutex
}

func CreateTCPClient(conn net.Conn) *TCPClient {
	return &TCPClient{
		TimeStamp:   time.Now(),
		Conn:        conn,
		Interactive: false,
		Group:       false,
		Hash:        hash.MD5(conn.RemoteAddr().String()),
		OS:          Unknown,
		Python2:     "",
		Python3:     "",
		User:        "",
		ReadLock:    new(sync.Mutex),
		WriteLock:   new(sync.Mutex),
	}
}

func (c *TCPClient) Close() {
	log.Info("Closing client: %s", c.FullDesc())
	c.Conn.Close()
}

func (c *TCPClient) AsTable() {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"Hash", "Network", "OS", "User", "Time", "Group"})
	t.AppendRow([]interface{}{
		c.Hash,
		c.Conn.RemoteAddr().String(),
		c.OS.String(),
		c.User,
		humanize.Time(c.TimeStamp),
		c.Group,
	})
	t.Render()
}

func (c *TCPClient) OnelineDesc() string {
	addr := c.Conn.RemoteAddr()
	return fmt.Sprintf("[%s] %s://%s (%s)", c.Hash, addr.Network(), addr.String(), c.OS.String())
}

func (c *TCPClient) FullDesc() string {
	addr := c.Conn.RemoteAddr()
	return fmt.Sprintf("[%s] %s://%s (connected at: %s) [%s] [%t]", c.Hash, addr.Network(), addr.String(),
		humanize.Time(c.TimeStamp), c.OS.String(), c.Group)
}

func (c *TCPClient) ReadUntilClean(token string) string {
	inputBuffer := make([]byte, 1)
	var outputBuffer bytes.Buffer
	for {
		c.ReadLock.Lock()
		n, err := c.Conn.Read(inputBuffer)
		c.ReadLock.Unlock()
		if err != nil {
			log.Error("Read from client failed")
			c.Interactive = false
			Ctx.DeleteTCPClient(c)
			return outputBuffer.String()
		}
		outputBuffer.Write(inputBuffer[:n])
		// If found token, then finish reading
		if strings.HasSuffix(outputBuffer.String(), token) {
			break
		}
	}
	// log.Debug("%d bytes read from client", len(outputBuffer.String()))
	return outputBuffer.String()[:len(outputBuffer.String())-len(token)]
}

func (c *TCPClient) ReadUntil(token string) (string, bool) {
	// Set read time out
	c.Conn.SetReadDeadline(time.Now().Add(time.Second * 3))

	inputBuffer := make([]byte, 1)
	var outputBuffer bytes.Buffer
	var isTimeout bool
	for {
		c.ReadLock.Lock()
		n, err := c.Conn.Read(inputBuffer)
		c.ReadLock.Unlock()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Error("Read response timeout from client")
				isTimeout = true
			} else {
				log.Error("Read from client failed")
				c.Interactive = false
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
	// log.Info("%d bytes read from client", len(outputBuffer.String()))
	return outputBuffer.String(), isTimeout
}

func (c *TCPClient) ReadSize(size int) string {
	c.Conn.SetReadDeadline(time.Now().Add(time.Second * 3))
	readSize := 0
	inputBuffer := make([]byte, 1)
	var outputBuffer bytes.Buffer
	for {
		c.ReadLock.Lock()
		n, err := c.Conn.Read(inputBuffer)
		c.ReadLock.Unlock()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Error("Read response timeout from client")
			} else {
				log.Error("Read from client failed")
				c.Interactive = false
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
	// log.Info("(%d/%d) bytes read from client", len(outputBuffer.String()), size)
	return outputBuffer.String()
}

func (c *TCPClient) Read(timeout time.Duration) (string, bool) {
	// Set read time out
	c.Conn.SetReadDeadline(time.Now().Add(timeout))

	inputBuffer := make([]byte, 0x400)
	var outputBuffer bytes.Buffer
	var isTimeout bool
	for {
		c.ReadLock.Lock()
		n, err := c.Conn.Read(inputBuffer)
		c.ReadLock.Unlock()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				isTimeout = true
			} else {
				log.Error("Read from client failed")
				c.Interactive = false
				Ctx.DeleteTCPClient(c)
				isTimeout = false
			}
			break
		}
		outputBuffer.Write(inputBuffer[:n])
	}
	// Reset read time out
	c.Conn.SetReadDeadline(time.Time{})

	return outputBuffer.String(), isTimeout
}

func (c *TCPClient) Write(data []byte) int {
	c.WriteLock.Lock()
	n, err := c.Conn.Write(data)
	c.WriteLock.Unlock()
	if err != nil {
		log.Error("Write to client failed")
		c.Interactive = false
		Ctx.DeleteTCPClient(c)
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
		if c.OS == Linux {
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
				return 0, errors.New("No python on target machine")
			}
		} else if c.OS == Windows {
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
				return 0, errors.New("No python on target machine")
			}
		} else {
			return 0, fmt.Errorf("Unsupported OS: %s", c.OS)
		}
	} else {
		return 0, errors.New("No such file")
	}
}

func (c *TCPClient) ReadfileEx(filename string, start int, length int) (string, error) {
	exists, err := c.FileExists(filename)
	if err != nil {
		return "", err
	}
	if exists {
		if c.OS == Linux {
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
				return c.Readfile(filename)
			}
		} else if c.OS == Windows {
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
				return c.Readfile(filename)
			}
		} else {
			return "", fmt.Errorf("Unsupported OS: %s", c.OS)
		}

	} else {
		return "", errors.New("No such file")
	}
}

func (c *TCPClient) Readfile(filename string) (string, error) {
	exists, err := c.FileExists(filename)
	if err != nil {
		return "", err
	}

	if exists {
		if c.OS == Linux {
			return c.SystemToken("cat " + filename), nil
		}
		return c.SystemToken("type " + filename), nil
	} else {
		return "", errors.New("No such file")
	}
}

func (c *TCPClient) FileExists(path string) (bool, error) {
	python := c.SelectPython()
	switch c.OS {
	case Linux:
		return c.SystemToken("ls "+path) == path+"\n", nil
	case Windows:
		if python != "" {
			// Python2 and Python3 all works fine in this situation
			command := fmt.Sprintf(
				"%s -c \"print(__import__('os').path.exists(__import__('base64').b64decode(b'%s')))\"",
				python,
				base64.StdEncoding.EncodeToString([]byte(path)),
			)
			return strings.TrimSpace(c.SystemToken(command)) == "True", nil
		} else {
			return false, errors.New("No python on the target machine")
		}
	default:
		return false, errors.New("Unrecognized operating system")
	}
}

func (c *TCPClient) System(command string) {
	c.Conn.Write([]byte(command + "\n"))
}

func (c *TCPClient) SystemToken(command string) string {
	tokenA := str.RandomString(0x10)
	tokenB := str.RandomString(0x10)

	var input string

	// Construct command to execute
	// ; echo tokenB and & echo tokenB are for commands which will be execute unsuccessfully
	if c.OS == Windows {
		// For Windows client
		input = "echo " + tokenA + " && " + command + " & echo " + tokenB
	} else {
		// For Linux client
		input = "echo " + tokenA + " && " + command + " ; echo " + tokenB
	}

	c.System(input)

	var isTimeout bool
	if c.OS == Windows {
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

func (c *TCPClient) DetectUser() {
	switch c.OS {
	case Linux:
		c.User = strings.TrimSpace(c.SystemToken("whoami"))
		log.Info("[%s] User detected: %s", c.Hash, c.User)
	case Windows:
		c.User = strings.TrimSpace(c.SystemToken("whoami"))
		log.Info("[%s] User detected: %s", c.Hash, c.User)
	default:
		log.Error("Unrecognized operating system")
	}
}

func (c *TCPClient) DetectPython() {
	var result string
	var version string
	if c.OS == Windows {
		// On windows platform, there is a fake python interpreter:
		// %HOME%\AppData\Local\Microsoft\WindowsApps\python.exe
		// The windows app store will be opened if the user didn't install python from the store
		// This situation will be fuzzy to us.
		result = strings.TrimSpace(c.SystemToken("where python"))
		if strings.HasSuffix(result, "python.exe") {
			version = strings.TrimSpace(c.SystemToken("python --version"))
			if strings.HasPrefix(version, "Python 3") {
				c.Python3 = strings.TrimSpace(strings.Split(result, "\n")[0])
				log.Info("Python3 found: %s", c.Python3)
				result = strings.TrimSpace(c.SystemToken("where python2"))
				if strings.HasSuffix(result, "python2.exe") {
					c.Python2 = strings.TrimSpace(strings.Split(result, "\n")[0])
					log.Info("Python2 found: %s", result)
				}
			} else if strings.HasPrefix(version, "Python 2") {
				c.Python2 = strings.TrimSpace(strings.Split(result, "\n")[0])
				log.Info("Python2 found: %s", c.Python2)
				result = strings.TrimSpace(c.SystemToken("where python3"))
				if strings.HasSuffix(result, "python3.exe") {
					c.Python3 = strings.TrimSpace(strings.Split(result, "\n")[0])
					log.Info("Python3 found: %s", result)
				}
			} else {
				log.Error("Unrecognized python version: %s", version)
			}
		} else {
			log.Error("No python on traget machine.")
		}
	} else if c.OS == Linux {
		result = strings.TrimSpace(c.SystemToken("which python2"))
		if result != "" {
			c.Python2 = strings.TrimSpace(strings.Split(result, "\n")[0])
			log.Info("Python2 found: %s", result)
		}
		result = strings.TrimSpace(c.SystemToken("which python3"))
		if result != "" {
			c.Python3 = strings.TrimSpace(strings.Split(result, "\n")[0])
			log.Info("Python3 found: %s", result)
		}
	} else {
		log.Error("Unknown OS: %s", c.OS.String())
	}
}

func (c *TCPClient) DetectOS() {
	// For Unix-Like OSs
	c.System("uname")
	output, _ := c.Read(time.Second * 2)
	kwos := map[string]OperatingSystem{
		"linux":   Linux,
		"sunos":   SunOS,
		"freebsd": FreeBSD,
		"darwin":  MacOS,
	}
	for keyword, os := range kwos {
		if strings.Contains(strings.ToLower(output), keyword) {
			c.OS = os
			log.Info("[%s] OS detected: %s", c.Hash, c.OS.String())
			return
		}
	}

	// For Windows
	c.System("ver")
	output, _ = c.Read(time.Second * 2)
	if strings.Contains(strings.ToLower(output), "windows") {
		c.OS = Windows
		log.Info("[%s] OS detected: %s", c.Hash, c.OS.String())
		return
	}

	// Unknown OS
	log.Info("OS detection failed, set [%s] to `Unknown`", c.Hash)
	c.OS = Unknown
}
