package context

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/WangYihang/Platypus/lib/util/hash"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/str"
	humanize "github.com/dustin/go-humanize"
)

type TCPClient struct {
	TimeStamp   time.Time
	Conn        net.Conn
	Interactive bool
	Group       bool
	Hash        string
	OS          string
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
		ReadLock:    new(sync.Mutex),
		WriteLock:   new(sync.Mutex),
	}
}

func (c *TCPClient) Close() {
	log.Info("Closing client: %s", c.Desc())
	c.Conn.Close()
}

func (c *TCPClient) OnelineDesc() string {
	addr := c.Conn.RemoteAddr()
	return fmt.Sprintf("[%s] %s://%s", c.Hash, addr.Network(), addr.String())
}

func (c *TCPClient) Desc() string {
	addr := c.Conn.RemoteAddr()
	return fmt.Sprintf("[%s] %s://%s (connected at: %s) [%s] [%t]", c.Hash, addr.Network(), addr.String(),
		humanize.Time(c.TimeStamp), c.OS, c.Group)
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
	log.Debug("%d bytes read from client", len(outputBuffer.String()))
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
	log.Info("%d bytes read from client", len(outputBuffer.String()))
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
	log.Info("(%d/%d) bytes read from client", len(outputBuffer.String()), size)
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

func (c *TCPClient) Readfile(filename string) string {
	if c.FileExists(filename) {
		return c.SystemToken("cat " + filename)
	} else {
		log.Error("No such file")
		return ""
	}
}

func (c *TCPClient) FileExists(path string) bool {
	return c.SystemToken("ls "+path) == path+"\n"
}

func (c *TCPClient) System(command string) {
	c.Conn.Write([]byte(command + "\n"))
}

func (c *TCPClient) SystemToken(command string) string {
	tokenA := str.RandomString(0x10)
	tokenB := str.RandomString(0x10)

	var input string
	if c.OS == "Windows" {
		// For Windows client
		input = "echo " + tokenA + " && " + command + " & echo " + tokenB
	} else {
		// For Linux client
		input = "echo " + tokenA + " && " + command + "&& echo " + tokenB
	}
	log.Info("Executing: %s", input)
	c.System(input)

	var isTimeout bool
	if c.OS == "Windows" {
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
	log.Info(result)
	return result
}

func (c *TCPClient) DetectOS() {
	log.Info("Detect [%s] OS", c.Hash)

	c.System("uname")
	output, _ := c.Read(time.Second * 3)
	if strings.Contains(output, "Linux") {
		c.OS = "Linux"
		log.Info("[%s] OS is Linux", c.Hash)
		return
	}

	c.System("ver")
	output, _ = c.Read(time.Second * 3)
	if strings.Contains(output, "Windows") {
		c.OS = "Windows"
		log.Info("[%s] OS is Windows", c.Hash)
		return
	}

	// Unknown OS
	log.Info("Unknown OS, set [%s] to Linux in default", c.Hash)
	c.OS = "Linux"
}
