package model

import (
	"fmt"
	"net"
	"time"

	"github.com/WangYihang/Platypus/lib/util/hash"
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

func (c Client) Close() {
	c.Conn.Close()
}

func (c Client) Desc() string {
	addr := c.Conn.RemoteAddr()
	return fmt.Sprintf("[%s] %s://%s (connected at: %s) [%t]", c.Hash, addr.Network(), addr.String(), humanize.Time(c.TimeStamp), c.Interactive)
}
