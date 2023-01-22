package agent

import (
	"fmt"
)

type PlainUDPAgent struct {
	AgentConn
}

func (s *PlainUDPAgent) Setup() {
	s.Username = "root"
	s.IP = "1.1.1.1"
}

func (s *PlainUDPAgent) System(command string) string {
	return fmt.Sprintf("PlainUDPAgent<System>: %s", command)
}

func (s *PlainUDPAgent) Download(remotePath string, localPath string) {

}

func (s *PlainUDPAgent) Upload(localPath string, remotePath string) {

}

func (s *PlainUDPAgent) Handle() {
	s.Conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 26\r\n\r\nthis is plain tcp listener"))
	s.Conn.Close()
}
