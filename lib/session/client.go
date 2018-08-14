package session

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net"
	"time"

	log "github.com/WangYihang/Platypus/lib/utils/log"
)

type Client struct {
	ts          time.Time
	conn        net.Conn
	interactive bool
}

func CreateClient(conn net.Conn) *Client {
	return &Client{
		ts:          time.Now(),
		conn:        conn,
		interactive: false,
	}
}

func (c Client) Hash() string {
	return MD5(c.Desc())
}

func (c Client) Close() {
	log.Info(fmt.Sprintf("Stoping client: ", c.Desc()))
	c.conn.Close()
}

func (c Client) Desc() string {
	addr := c.conn.RemoteAddr()
	return fmt.Sprintf("%s://%s", addr.Network(), addr.String())
}

func MD5(data string) string {
	m := md5.New()
	m.Write([]byte(data))
	return hex.EncodeToString(m.Sum(nil))
}
