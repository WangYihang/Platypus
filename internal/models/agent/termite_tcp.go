package agent

import (
	"fmt"
)

type TermiteTCPAgent struct {
	AgentConn
}

func (s *TermiteTCPAgent) Setup() {
	s.Username = "root"
	s.IP = "1.1.1.1"
}

func (s *TermiteTCPAgent) System(command string) string {
	return fmt.Sprintf("TermiteTCPAgent<System>: %s", command)
}

func (s *TermiteTCPAgent) Download(remotePath string, localPath string) {

}

func (s *TermiteTCPAgent) Upload(localPath string, remotePath string) {

}
func (s *TermiteTCPAgent) Handle() {
	s.Conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 26\r\n\r\nthis is plain tcp listener"))
	s.Conn.Close()
}
