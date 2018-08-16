package model

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/lib/util/hash"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/str"
	humanize "github.com/dustin/go-humanize"
)

type Client struct {
	TimeStamp   time.Time
	Conn        net.Conn
	Interactive bool
	Hash        string
	InPipe      chan []byte
	OutPipe     chan []byte
}

func CreateClient(conn net.Conn) *Client {
	client := &Client{
		TimeStamp:   time.Now(),
		Conn:        conn,
		Interactive: false,
		Hash:        hash.MD5(conn.RemoteAddr().String()),
		InPipe:      make(chan []byte),
		OutPipe:     make(chan []byte),
	}
	return client
}

func (c *Client) Close() {
	log.Info("Closeing client: %s", c.Desc())
	c.Conn.Close()
}

func (c *Client) Desc() string {
	addr := c.Conn.RemoteAddr()
	return fmt.Sprintf("[%s] %s://%s (connected at: %s) [%t]", c.Hash, addr.Network(), addr.String(), humanize.Time(c.TimeStamp), c.Interactive)
}

func (c *Client) Readfile(filename string) string {
	if c.FileExists(filename) {
		return c.SystemToken("cat " + filename)
	} else {
		return ""
	}
}

func (c *Client) FileExists(path string) bool {
	return c.SystemToken("ls "+path) == path+"\n"
}

func (c *Client) System(command string) {
	c.Conn.Write([]byte(command + "\n"))
}

func (c *Client) SystemToken(command string) string {
	tokenA := str.RandomString(0x10)
	tokenB := str.RandomString(0x10)
	input := "echo " + tokenA + " && " + command + "; echo " + tokenB
	c.System(input)
	c.ReadUntil(tokenA)
	output := c.ReadUntil(tokenB)
	log.Info(output)
	return output
}

func (c *Client) ReadUntil(token string) string {
	inputBuffer := make([]byte, 1)
	var outputBuffer bytes.Buffer
	log.Info("Start loop")
	for {
		n, err := c.Conn.Read(inputBuffer)
		log.Info("%d bytes read", n)
		if err != nil {
			return outputBuffer.String()
		}
		log.Info(string(inputBuffer[:n]))
		log.Info("Save to buffer: %s", string(inputBuffer[:n]))
		outputBuffer.Write(inputBuffer[:n])
		if strings.HasSuffix(outputBuffer.String(), token) {
			return outputBuffer.String()
		}
	}
}

func (c *Client) ReadAll() string {
	inputBuffer := make([]byte, 1024)
	var outputBuffer bytes.Buffer
	for {
		n, err := c.Conn.Read(inputBuffer)
		if err != nil {
			return outputBuffer.String()
		}
		if n == 0 {
			break
		}
		outputBuffer.Write(inputBuffer[:n])
	}
	return outputBuffer.String()
}

func (c *Client) Read() {
	for {
		buffer := make([]byte, 1024)
		_, err := c.Conn.Read(buffer)
		if err != nil {
			log.Error("Read failed from %s , error message: %s", c.Desc(), err)
			close(c.OutPipe)
			Ctx.DeleteClient(c)
			return
		}
		c.OutPipe <- buffer
	}
}

func (c *Client) Write() {
	for {
		select {
		case data, ok := <-c.InPipe:
			if !ok {
				log.Error("Channel of %s closed", c.Desc())
				close(c.InPipe)
				Ctx.DeleteClient(c)
				return
			}
			n, err := c.Conn.Write(data)
			if err != nil {
				log.Error("Write failed to %s , error message: %s", c.Desc(), err)
				close(c.InPipe)
				Ctx.DeleteClient(c)
				return
			}
			log.Info("%d bytes sent", n)
		}
	}
}
