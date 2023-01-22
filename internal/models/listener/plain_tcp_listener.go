package listener

import (
	"fmt"
	"net"
	"strconv"

	"github.com/WangYihang/Platypus/internal/databases"
	"github.com/WangYihang/Platypus/internal/models/agent"
	"github.com/getsentry/sentry-go"
)

type PlainTCPListener struct {
	Listener
	stopChan chan bool
	conn     chan net.Conn
}

func (l *PlainTCPListener) Enable() (err error) {
	fmt.Println("Enabling Plain TCP Listener")
	// Parse address
	address, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", l.Host, l.Port))
	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	// Create listener
	listener, err := net.ListenTCP("tcp", address)
	if err != nil {
		sentry.CaptureException(err)
		fmt.Println(err.Error())
		return err
	}
	// Start goroutine to accept new connections
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				fmt.Println(err.Error())
				break
			}
			l.conn <- conn
		}
	}()
	// Start goroutine to handle connections
	go func() {
		for {
			select {
			case <-l.stopChan:
				listener.Close()
				return
			case conn := <-l.conn:
				l.Handle(conn)
				databases.DB.Model(&l.Listener).
					Where("ID = ?", l.ID).
					Update("num_agents", l.NumAgents+1)
			}
		}
	}()
	return nil
}

func (l *PlainTCPListener) Disable() error {
	fmt.Println("Disabling Plain TCP Listener")
	if l.Listener.Enable {
		l.stopChan <- true
		return nil
	} else {
		return fmt.Errorf("already disabled")
	}
}

func (l *PlainTCPListener) Handle(conn net.Conn) {
	os := "linux"
	arch := "arm"
	host, port_str, _ := net.SplitHostPort(conn.RemoteAddr().String())
	port, _ := strconv.Atoi(port_str)
	agent.CreateAgent(os, arch, host, uint16(port), "", "", "", "plain_tcp", "", "", conn)
}
