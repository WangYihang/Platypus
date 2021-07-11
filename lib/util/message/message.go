package message

import (
	"encoding/gob"
)

type MessageType int

const (
	// Platypus <-> Termite
	STDIO MessageType = iota
	PULL_TUNNEL_DATA

	// Platypus -> Termite
	WINDOW_SIZE
	GET_CLIENT_INFO
	DUPLICATED_CLIENT
	PROCESS_START
	PROCESS_TERMINATE
	Pull_TUNNEL_CREATE
	PULL_TUNNEL_CONNECT
	PULL_TUNNEL_DISCONNECT

	// Termite -> Platypus
	PROCESS_STARTED
	PROCESS_STOPED
	CLIENT_INFO
	PULL_TUNNEL_CONNECTED
	PULL_TUNNEL_CONNECT_FAILED
	PULL_TUNNEL_DISCONNECTED
)

type Message struct {
	Type MessageType
	Body interface{}
}

type BodyStdio struct {
	Key  string
	Data []byte
}

type BodyWindowSize struct {
	Key     string
	Columns int
	Rows    int
}

type BodyStartProcess struct {
	Key           string
	Path          string
	WindowColumns int
	WindowRows    int
}

type BodyProcessStarted struct {
	Key string
	Pid int
}

type BodyProcessStoped struct {
	Key  string
	Code int
}

type BodyGetClientInfo struct{}

type BodyDuplicateClient struct{}

type BodyClientInfo struct {
	OS                string
	User              string
	Python2           string
	Python3           string
	NetworkInterfaces map[string]string
}

type BodyTerminateProcess struct {
	Key string
}

type BodyPullTunnelConnect struct {
	Token   string
	Address string
}

type BodyPullTunnelConnected struct {
	Token string
}

type BodyPullTunnelConnectFailed struct {
	Token  string
	Reason string
}

type BodyPullTunnelDisconnect struct {
	Token string
}

type BodyPullTunnelDisconnected struct {
	Token string
}

type BodyPullTunnelData struct {
	Token string
	Data  []byte
}

func RegisterGob() {
	gob.Register(&BodyStdio{})
	gob.Register(&BodyWindowSize{})
	gob.Register(&BodyStartProcess{})
	gob.Register(&BodyProcessStarted{})
	gob.Register(&BodyProcessStoped{})
	gob.Register(&BodyGetClientInfo{})
	gob.Register(&BodyDuplicateClient{})
	gob.Register(&BodyClientInfo{})
	gob.Register(&BodyTerminateProcess{})
	gob.Register(&BodyPullTunnelConnect{})
	gob.Register(&BodyPullTunnelConnected{})
	gob.Register(&BodyPullTunnelConnectFailed{})
	gob.Register(&BodyPullTunnelDisconnect{})
	gob.Register(&BodyPullTunnelDisconnected{})
	gob.Register(&BodyPullTunnelData{})
}
