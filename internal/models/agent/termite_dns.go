package agent

import (
	"fmt"
)

type TermiteDNSAgent struct {
	AgentConn
}

func (s *TermiteDNSAgent) Setup() {
	s.Username = "root"
	s.IP = "1.1.1.1"
}

func (s *TermiteDNSAgent) System(command string) string {
	return fmt.Sprintf("TermiteDNSAgent<System>: %s", command)
}

func (s *TermiteDNSAgent) Download(remotePath string, localPath string) {

}

func (s *TermiteDNSAgent) Upload(localPath string, remotePath string) {

}

func (s *TermiteDNSAgent) Handle() {
	s.Conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 26\r\n\r\nthis is plain tcp listener"))
	s.Conn.Close()
}
