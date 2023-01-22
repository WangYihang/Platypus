package agent

import (
	"fmt"
)

type TermiteICMPAgent struct {
	AgentConn
}

func (s *TermiteICMPAgent) Setup() {
	s.Username = "root"
	s.IP = "1.1.1.1"
}

func (s *TermiteICMPAgent) System(command string) string {
	return fmt.Sprintf("TermiteICMPAgent<System>: %s", command)
}

func (s *TermiteICMPAgent) Download(remotePath string, localPath string) {

}

func (s *TermiteICMPAgent) Upload(localPath string, remotePath string) {

}
func (s *TermiteICMPAgent) Handle() {
	s.Conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 26\r\n\r\nthis is plain tcp listener"))
	s.Conn.Close()
}
