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
	Pipe        chan []byte
}

func CreateClient(conn net.Conn) *Client {
	return &Client{
		TimeStamp:   time.Now(),
		Conn:        conn,
		Interactive: false,
		Hash:        hash.MD5(conn.RemoteAddr().String()),
		Pipe:        make(chan []byte),
	}
}

func (c Client) Close() {
	log.Info(fmt.Sprintf("Stoping client: %s", c.Desc()))
	c.Conn.Close()
}

func (c Client) Desc() string {
	addr := c.Conn.RemoteAddr()
	return fmt.Sprintf("[%s] %s://%s (connected at: %s) [%t]", c.Hash, addr.Network(), addr.String(), humanize.Time(c.TimeStamp), c.Interactive)
}
