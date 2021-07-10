package message

import (
	"encoding/gob"
)

type MessageType int

const (
	// Platypus <-> Termite
	STDIO MessageType = iota

	// Platypus -> Termite
	WINDOW_SIZE
	GET_CLIENT_INFO
	DUPLICATED_CLIENT
	START_PROCESS
	TERMINATE_PROCESS

	// Termite -> Platypus
	PROCESS_STARTED
	PROCESS_STOPED
	CLIENT_INFO

	// Tunnel
	TUNNEL_CONNECT
	TUNNEL_CONNECTED
	TUNNEL_DATA
	TUNNEL_DISCONNECT
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
	Target string
}
type BodyTunnelConnected struct {
	Target string
}
type BodyTunnelData struct {
	Target string
	Data   []byte
}

type BodyTunnelDisconnect struct {
	Target string
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
	gob.Register(&BodyTunnelData{})
	gob.Register(&BodyTunnelDisconnect{})
}
