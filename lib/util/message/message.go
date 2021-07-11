package message

import (
	"encoding/gob"
)

type MessageType int

const (
	// Platypus <-> Termite
	STDIO MessageType = iota
	TUNNEL_DATA

	// Platypus -> Termite
	WINDOW_SIZE
	GET_CLIENT_INFO
	DUPLICATED_CLIENT
	PROCESS_START
	PROCESS_TERMINATE
	TUNNEL_CREATE
	TUNNEL_DELETE
	TUNNEL_CONNECT
	TUNNEL_DISCONNECT

	// Termite -> Platypus
	PROCESS_STARTED
	PROCESS_STOPED
	CLIENT_INFO
	TUNNEL_CONNECTED
	TUNNEL_CONNECT_FAILED
	TUNNEL_DISCONNECTED
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

type BodyTunnelConnect struct {
	Token   string
	Address string
}

type BodyTunnelConnected struct {
	Token string
}

type BodyTunnelConnectFailed struct {
	Token  string
	Reason string
}

type BodyTunnelDisconnect struct {
	Token string
}

type BodyTunnelDisconnected struct {
	Token string
}

type BodyTunnelData struct {
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
	gob.Register(&BodyTunnelConnect{})
	gob.Register(&BodyTunnelConnected{})
	gob.Register(&BodyTunnelConnectFailed{})
	gob.Register(&BodyTunnelDisconnect{})
	gob.Register(&BodyTunnelDisconnected{})
	gob.Register(&BodyTunnelData{})
}
