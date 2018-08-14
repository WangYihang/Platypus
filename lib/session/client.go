package session

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net"
	"time"

	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/dustin/go-humanize"
)

type Client struct {
	TimeStamp   time.Time
	Conn        net.Conn
	Interactive bool
	Hash        string
}

func CreateClient(conn net.Conn) *Client {
	return &Client{
		TimeStamp:   time.Now(),
		Conn:        conn,
		Interactive: false,
		Hash:        MD5(conn.RemoteAddr().String()),
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

func MD5(data string) string {
	m := md5.New()
	m.Write([]byte(data))
	return hex.EncodeToString(m.Sum(nil))
}
