package model

import (
	"fmt"
	"net"
	"time"

	"github.com/WangYihang/Platypus/lib/util/hash"
	"github.com/WangYihang/Platypus/lib/util/log"
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
	go client.Read()
	go client.Write()
	return client
}

func (c Client) Close() {
	log.Info(fmt.Sprintf("Stopping client: %s", c.Desc()))
	c.Conn.Close()
}

func (c Client) Read() {
	for {
		buffer := make([]byte, 1024)
		n, err := c.Conn.Read(buffer)
		if err != nil {
			log.Error("Read failed from %s , error message: %s", c.Desc(), err)
			c.Close()
			return
		}
		log.Info("%d bytes recieved", n)
		c.OutPipe <- buffer
	}
}

func (c Client) Write() {
	for {
		select {
		case data := <-c.InPipe:
			n, err := c.Conn.Write(data)
			if err != nil {
				log.Error("Write failed to %s , error message: %s", c.Desc(), err)
				c.Close()
				return
			}
			log.Info("%d bytes sent", n)
		}
	}
}

func (c Client) Desc() string {
	addr := c.Conn.RemoteAddr()
	return fmt.Sprintf("[%s] %s://%s (connected at: %s) [%t]", c.Hash, addr.Network(), addr.String(), humanize.Time(c.TimeStamp), c.Interactive)
}
