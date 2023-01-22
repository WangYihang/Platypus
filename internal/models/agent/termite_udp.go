package agent

import (
	"fmt"
)

type TermiteUDPAgent struct {
	AgentConn
}

func (s *TermiteUDPAgent) Setup() {
	s.Username = "root"
	s.IP = "1.1.1.1"
}

func (s *TermiteUDPAgent) System(command string) string {
	return fmt.Sprintf("TermiteUDPAgent<System>: %s", command)
}

func (s *TermiteUDPAgent) Download(remotePath string, localPath string) {

}

func (s *TermiteUDPAgent) Upload(localPath string, remotePath string) {

}
func (s *TermiteUDPAgent) Handle() {
	s.Conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 26\r\n\r\nthis is plain tcp listener"))
	s.Conn.Close()
}
