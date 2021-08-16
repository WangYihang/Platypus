package message

import (
	"encoding/gob"
)

type MessageType int

const (
	// Platypus -> Termite
	GET_CLIENT_INFO MessageType = iota // DO NOT CHANGE OR PREPEND OPCODE BRFORE THIS OPCODE
	UPDATE

	WINDOW_SIZE
	DUPLICATED_CLIENT
	PROCESS_START
	PROCESS_TERMINATE
	PULL_TUNNEL_CONNECT
	PULL_TUNNEL_DISCONNECT
	PUSH_TUNNEL_CREATE
	PUSH_TUNNEL_DELETE
	PUSH_TUNNEL_CONNECTED
	PUSH_TUNNEL_CONNECT_FAILED
	PUSH_TUNNEL_DISCONNECTED
	PUSH_TUNNEL_DISCONNECT_FAILED
	DYNAMIC_TUNNEL_CREATE
	DYNAMIC_TUNNEL_DESTROY
	CALL_SYSTEM
	READ_FILE
	READ_FILE_EX
	FILE_SIZE
	WRITE_FILE
	WRITE_FILE_EX

	// Termite -> Platypus
	CLIENT_INFO
	PROCESS_STARTED
	PROCESS_STOPED
	PULL_TUNNEL_CONNECTED
	PULL_TUNNEL_CONNECT_FAILED
	PULL_TUNNEL_DISCONNECTED
	PUSH_TUNNEL_CONNECT
	PUSH_TUNNEL_DISCONNECT
	PUSH_TUNNEL_CREATED
	PUSH_TUNNEL_CREATE_FAILED
	PUSH_TUNNEL_DELETED
	PUSH_TUNNEL_DELETE_FAILED
	DYNAMIC_TUNNEL_CREATED
	DYNAMIC_TUNNEL_CREATE_FAILED
	DYNAMIC_TUNNEL_DESTROIED
	DYNAMIC_TUNNEL_DESTROY_FAILED
	CALL_SYSTEM_RESULT
	READ_FILE_RESULT
	READ_FILE_EX_RESULT
	FILE_SIZE_RESULT
	WRITE_FILE_RESULT
	WRITE_FILE_EX_RESULT

	// Platypus <-> Termite
	STDIO
	PULL_TUNNEL_DATA
	PUSH_TUNNEL_DATA
)

type Message struct {
	Type MessageType
	Body interface{}
}

type BodyUpdate struct {
	DistributorURL string
	Version        string
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
	Version           string
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

type BodyPushTunnelData struct {
	Token string
	Data  []byte
}

type BodyPushTunnelCreate struct {
	Address string
}

type BodyPushTunnelCreated struct {
	Address string
}

type BodyPushTunnelCreateFailed struct {
	Address string
	Reason  string
}

type BodyPushTunnelDelete struct {
	Token string
}

type BodyPushTunnelDeleted struct {
	Token string
}

type BodyPushTunnelDeleteFailed struct {
	Token  string
	Reason string
}

type BodyPushTunnelConnect struct {
	Token   string
	Address string
}

type BodyPushTunnelConnected struct {
	Token string
}
type BodyPushTunnelConnectFailed struct {
	Token  string
	Reason string
}

type BodyPushTunnelDisonnect struct {
	Token string
}

type BodyPushTunnelDisonnected struct {
	Token  string
	Reason string
}

type BodyPushTunnelDisonnectFailed struct {
	Token string
}

type BodyDynamicTunnelCreate struct{}
type BodyDynamicTunnelCreated struct {
	Port int
}
type BodyDynamicTunnelCreateFailed struct {
	Reason string
}
type BodyDynamicTunnelDestroy struct{}
type BodyDynamicTunnelDestroied struct{}
type BodyDynamicTunnelDestroyFailed struct {
	Reason string
}

type BodyCallSystem struct {
	Token   string
	Command string
}

type BodyCallSystemResult struct {
	Token  string
	Result []byte
}

type BodyReadFile struct {
	Token string
	Path  string
}

type BodyReadFileResult struct {
	Token  string
	Result []byte
}

type BodyReadFileEx struct {
	Token string
	Path  string
	Start int64
	Size  int64
}

type BodyReadFileExResult struct {
	Token  string
	Result []byte
}

type BodyFileSize struct {
	Token string
	Path  string
}

type BodyFileSizeResult struct {
	Token string
	N     int64
}

type BodyWriteFile struct {
	Token   string
	Path    string
	Content []byte
}

type BodyWriteFileResult struct {
	Token string
	N     int
}

type BodyWriteFileEx struct {
	Token   string
	Path    string
	Content []byte
}

type BodyWriteFileExResult struct {
	Token string
	N     int
}

func RegisterGob() {
	// Client Management
	gob.Register(&BodyUpdate{})
	gob.Register(&BodyClientInfo{})
	gob.Register(&BodyGetClientInfo{})
	gob.Register(&BodyDuplicateClient{})
	// Process management
	gob.Register(&BodyStdio{})
	gob.Register(&BodyStartProcess{})
	gob.Register(&BodyProcessStarted{})
	gob.Register(&BodyProcessStoped{})
	gob.Register(&BodyTerminateProcess{})
	gob.Register(&BodyWindowSize{})
	// Local port forwarding
	gob.Register(&BodyPullTunnelConnect{})
	gob.Register(&BodyPullTunnelConnected{})
	gob.Register(&BodyPullTunnelConnectFailed{})
	gob.Register(&BodyPullTunnelDisconnect{})
	gob.Register(&BodyPullTunnelDisconnected{})
	gob.Register(&BodyPullTunnelData{})
	// Remote port forwarding
	gob.Register(&BodyPushTunnelData{})
	gob.Register(&BodyPushTunnelCreate{})
	gob.Register(&BodyPushTunnelCreated{})
	gob.Register(&BodyPushTunnelCreateFailed{})
	gob.Register(&BodyPushTunnelDelete{})
	gob.Register(&BodyPushTunnelDeleted{})
	gob.Register(&BodyPushTunnelDeleteFailed{})
	gob.Register(&BodyPushTunnelConnect{})
	gob.Register(&BodyPushTunnelConnected{})
	gob.Register(&BodyPushTunnelConnectFailed{})
	gob.Register(&BodyPushTunnelDisonnect{})
	gob.Register(&BodyPushTunnelDisonnected{})
	gob.Register(&BodyPushTunnelDisonnectFailed{})
	// Dynamic port forwarding
	gob.Register(&BodyDynamicTunnelCreate{})
	gob.Register(&BodyDynamicTunnelCreated{})
	gob.Register(&BodyDynamicTunnelCreateFailed{})
	gob.Register(&BodyDynamicTunnelDestroy{})
	gob.Register(&BodyDynamicTunnelDestroied{})
	gob.Register(&BodyDynamicTunnelDestroyFailed{})
	// System
	gob.Register(&BodyCallSystem{})
	gob.Register(&BodyCallSystemResult{})
	// Read file
	gob.Register(&BodyReadFile{})
	gob.Register(&BodyReadFileResult{})
	gob.Register(&BodyReadFileEx{})
	gob.Register(&BodyReadFileExResult{})
	// Get file size
	gob.Register(&BodyFileSize{})
	gob.Register(&BodyFileSizeResult{})
	// Write file
	gob.Register(&BodyWriteFile{})
	gob.Register(&BodyWriteFileResult{})
	// Append file
	gob.Register(&BodyWriteFileEx{})
	gob.Register(&BodyWriteFileExResult{})
}
