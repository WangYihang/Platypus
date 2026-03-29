package agent

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"syscall"

	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/utils/message"
	"github.com/WangYihang/Platypus/internal/utils/str"
	"github.com/WangYihang/Platypus/internal/utils/update"
	"github.com/armon/go-socks5"
	"github.com/creack/pty"
	"github.com/phayes/freeport"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
)

// HandleConnection runs the main message dispatch loop for the agent.
func HandleConnection(c *Client, state *State) {
	for {
		msg := &message.Message{}
		c.DecoderLock.Lock()
		err := c.Decoder.Decode(msg)
		c.DecoderLock.Unlock()
		if err != nil {
			log.Error("Network error: %s", err)
			break
		}
		switch msg.Type {
		case message.STDIO:
			if p, ok := state.Processes.Get(msg.Body.(*message.BodyStdio).Key); ok {
				p.Ptmx.Write(msg.Body.(*message.BodyStdio).Data)
			}
		case message.WINDOW_SIZE:
			ws := &pty.Winsize{
				Cols: uint16(msg.Body.(*message.BodyWindowSize).Columns),
				Rows: uint16(msg.Body.(*message.BodyWindowSize).Rows),
			}
			if p, ok := state.Processes.Get(msg.Body.(*message.BodyWindowSize).Key); ok {
				pty.Setsize(p.Ptmx, ws)
			}
		case message.PROCESS_START:
			handleProcessStart(c, state, msg)
		case message.PROCESS_STARTED:
		case message.PROCESS_STOPED:
		case message.GET_CLIENT_INFO:
			handleGetClientInfo(c)
		case message.DUPLICATED_CLIENT:
			log.Error("Duplicated connection")
			os.Exit(0)
		case message.PROCESS_TERMINATE:
			handleProcessTerminate(state, msg)
		case message.PULL_TUNNEL_CONNECT:
			handlePullTunnelConnect(c, state, msg)
		case message.PULL_TUNNEL_DATA:
			handlePullTunnelData(state, msg)
		case message.PULL_TUNNEL_DISCONNECT:
			handlePullTunnelDisconnect(c, state, msg)
		case message.PUSH_TUNNEL_CREATE:
			handlePushTunnelCreate(c, state, msg)
		case message.PUSH_TUNNEL_CONNECTED:
			handlePushTunnelConnected(c, state, msg)
		case message.PUSH_TUNNEL_DISCONNECTED:
			token := msg.Body.(*message.BodyPushTunnelDisonnected).Token
			if conn, ok := state.PushTunnels.GetAndDelete(token); ok {
				(*conn).Close()
			}
		case message.PUSH_TUNNEL_CONNECT_FAILED:
			token := msg.Body.(*message.BodyPushTunnelConnectFailed).Token
			if conn, ok := state.PushTunnels.GetAndDelete(token); ok {
				(*conn).Close()
			}
		case message.PUSH_TUNNEL_DATA:
			handlePushTunnelData(state, msg)
		case message.DYNAMIC_TUNNEL_CREATE:
			handleDynamicTunnelCreate(c, state)
		case message.DYNAMIC_TUNNEL_DESTROY:
			handleDynamicTunnelDestroy(c, state)
		case message.CALL_SYSTEM:
			handleCallSystem(c, msg)
		case message.READ_FILE:
			handleReadFile(c, msg)
		case message.READ_FILE_EX:
			handleReadFileEx(c, msg)
		case message.FILE_SIZE:
			handleFileSize(c, msg)
		case message.WRITE_FILE:
			handleWriteFile(c, msg)
		case message.WRITE_FILE_EX:
			handleWriteFileEx(c, msg)
		case message.UPDATE:
			handleUpdate(c, msg)
		}
	}
}

func sendMsg(c *Client, msg message.Message) error {
	c.EncoderLock.Lock()
	defer c.EncoderLock.Unlock()
	if err := c.Encoder.Encode(msg); err != nil {
		log.Error("Encode error: %s", err)
		return err
	}
	return nil
}

func handleProcessStart(c *Client, state *State, msg *message.Message) {
	body := msg.Body.(*message.BodyStartProcess)
	if body.Path == "" {
		return
	}
	log.Success("Starting process: %s", body.Path)
	process := exec.Command(body.Path)
	process.Env = os.Environ()
	process.Env = append(process.Env, "HISTORY=", "HISTSIZE=0", "HISTSAVE=",
		"HISTZONE=", "HISTLOG=", "HISTFILE=", "HISTFILE=/dev/null", "HISTFILESIZE=0")

	ws := pty.Winsize{
		Rows: uint16(body.WindowRows),
		Cols: uint16(body.WindowColumns),
	}
	ptmx, _ := pty.StartWithSize(process, &ws)
	state.Processes.Set(body.Key, &TermiteProcess{
		WindowSize: &ws,
		Ptmx:       ptmx,
		Process:    process,
	})
	log.Success("Process started: %d", process.Process.Pid)
	defer func() { _ = ptmx.Close() }()

	if err := sendMsg(c, message.Message{
		Type: message.PROCESS_STARTED,
		Body: message.BodyProcessStarted{Key: body.Key, Pid: process.Process.Pid},
	}); err != nil {
		return
	}

	go func() {
		for {
			buffer := make([]byte, 0x4000)
			n, err := ptmx.Read(buffer)
			if err != nil {
				if err == io.EOF {
					sendMsg(c, message.Message{
						Type: message.PROCESS_STOPED,
						Body: message.BodyProcessStoped{Key: body.Key, Code: 0},
					})
				}
				break
			}
			if n > 0 {
				if err := sendMsg(c, message.Message{
					Type: message.STDIO,
					Body: message.BodyStdio{Key: body.Key, Data: buffer[0:n]},
				}); err != nil {
					break
				}
			}
		}
	}()

	go func() {
		err := process.Wait()
		exitCode := 0
		if err != nil {
			exitCode = err.(*exec.ExitError).ExitCode()
		}
		fmt.Println("Exit code: ", exitCode)
		sendMsg(c, message.Message{
			Type: message.PROCESS_STOPED,
			Body: message.BodyProcessStoped{Key: body.Key, Code: exitCode},
		})
		state.Processes.Delete(body.Key)
	}()
}

func handleGetClientInfo(c *Client) {
	userInfo, err := user.Current()
	username := "Unknown"
	if err == nil {
		username = userInfo.Username
	}
	python2, _ := exec.LookPath("python2")
	python3, _ := exec.LookPath("python3")

	interfaces := map[string]string{}
	ifaces, _ := net.Interfaces()
	for _, i := range ifaces {
		interfaces[i.Name] = i.HardwareAddr.String()
	}

	if err := sendMsg(c, message.Message{
		Type: message.CLIENT_INFO,
		Body: message.BodyClientInfo{
			Version:           update.Version,
			User:              username,
			OS:                runtime.GOOS,
			Python2:           python2,
			Python3:           python3,
			NetworkInterfaces: interfaces,
		},
	}); err != nil {
		return
	}
}

func handleProcessTerminate(state *State, msg *message.Message) {
	key := msg.Body.(*message.BodyTerminateProcess).Key
	log.Success("Request terminate %s", key)
	if p, ok := state.Processes.Get(key); ok {
		if proc, err := os.FindProcess(p.Process.Process.Pid); err != nil {
			log.Error("Unable to find process: %s", err)
		} else {
			proc.Signal(syscall.SIGTERM)
			p.Ptmx.Close()
		}
	}
}

func handlePullTunnelConnect(c *Client, state *State, msg *message.Message) {
	address := msg.Body.(*message.BodyPullTunnelConnect).Address
	token := msg.Body.(*message.BodyPullTunnelConnect).Token

	conn, err := net.Dial("tcp", address)
	if err != nil {
		log.Error(err.Error())
		sendMsg(c, message.Message{
			Type: message.PULL_TUNNEL_CONNECT_FAILED,
			Body: message.BodyPullTunnelConnectFailed{Token: token, Reason: err.Error()},
		})
		return
	}

	if err := sendMsg(c, message.Message{
		Type: message.PULL_TUNNEL_CONNECTED,
		Body: message.BodyPullTunnelConnected{Token: token},
	}); err != nil {
		conn.Close()
		return
	}

	state.PullTunnels.Set(token, &conn)
	go func() {
		for {
			buffer := make([]byte, 0x100)
			n, err := conn.Read(buffer)
			if err != nil {
				log.Success("Tunnel (%s) disconnected: %s", token, err.Error())
				sendMsg(c, message.Message{
					Type: message.PULL_TUNNEL_DISCONNECTED,
					Body: message.BodyPullTunnelDisconnected{Token: token},
				})
				conn.Close()
				break
			}
			if n > 0 {
				sendMsg(c, message.Message{
					Type: message.PULL_TUNNEL_DATA,
					Body: message.BodyPullTunnelData{Token: token, Data: buffer[0:n]},
				})
			}
		}
	}()
}

func handlePullTunnelData(state *State, msg *message.Message) {
	token := msg.Body.(*message.BodyPullTunnelData).Token
	data := msg.Body.(*message.BodyPullTunnelData).Data
	if conn, ok := state.PullTunnels.Get(token); ok {
		if _, err := (*conn).Write(data); err != nil {
			(*conn).Close()
		}
	} else {
		log.Debug("No such tunnel: %s", token)
	}
}

func handlePullTunnelDisconnect(c *Client, state *State, msg *message.Message) {
	token := msg.Body.(*message.BodyPullTunnelDisconnect).Token
	if conn, ok := state.PullTunnels.GetAndDelete(token); ok {
		log.Info("Closing connection: %s", (*conn).RemoteAddr().String())
		(*conn).Close()
		sendMsg(c, message.Message{
			Type: message.PULL_TUNNEL_DISCONNECTED,
			Body: message.BodyPullTunnelDisconnected{Token: token},
		})
	} else {
		log.Debug("No such tunnel: %s", token)
	}
}

func handlePushTunnelCreate(c *Client, state *State, msg *message.Message) {
	address := msg.Body.(*message.BodyPushTunnelCreate).Address
	log.Info("Creating remote port forwarding from %s", address)
	server, err := net.Listen("tcp", address)
	if err != nil {
		log.Error("Server (%s) create failed: %s", address, err.Error())
		sendMsg(c, message.Message{
			Type: message.PUSH_TUNNEL_CREATE_FAILED,
			Body: message.BodyPushTunnelCreateFailed{Address: address, Reason: err.Error()},
		})
		return
	}
	log.Success("Server created (%s)", address)
	sendMsg(c, message.Message{
		Type: message.PUSH_TUNNEL_CREATED,
		Body: message.BodyPushTunnelCreated{Address: address},
	})
	go func() {
		for {
			conn, err := server.Accept()
			if err != nil {
				break
			}
			token := str.RandomString(0x10)
			log.Success("Connection came from: %s", conn.RemoteAddr().String())
			sendMsg(c, message.Message{
				Type: message.PUSH_TUNNEL_CONNECT,
				Body: message.BodyPushTunnelConnect{Token: token, Address: address},
			})
			state.PushTunnels.Set(token, &conn)
		}
	}()
}

func handlePushTunnelConnected(c *Client, state *State, msg *message.Message) {
	token := msg.Body.(*message.BodyPushTunnelConnected).Token
	log.Success("Connection (%s) connected", token)
	conn, ok := state.PushTunnels.Get(token)
	if !ok {
		log.Debug("No such tunnel (PUSH_TUNNEL_CONNECTED): %s", token)
		return
	}
	go func() {
		for {
			buffer := make([]byte, 0x4000)
			n, err := (*conn).Read(buffer)
			if err != nil {
				log.Error("Read from (%s) failed: %s", token, err.Error())
				sendMsg(c, message.Message{
					Type: message.PUSH_TUNNEL_DISCONNECTED,
					Body: message.BodyPushTunnelDisonnected{Token: token, Reason: err.Error()},
				})
				(*conn).Close()
				state.PushTunnels.Delete(token)
				break
			}
			log.Debug("%d bytes read from (%s)", n, token)
			sendMsg(c, message.Message{
				Type: message.PUSH_TUNNEL_DATA,
				Body: message.BodyPushTunnelData{Token: token, Data: buffer[0:n]},
			})
		}
	}()
}

func handlePushTunnelData(state *State, msg *message.Message) {
	token := msg.Body.(*message.BodyPushTunnelData).Token
	data := msg.Body.(*message.BodyPushTunnelData).Data
	if conn, ok := state.PushTunnels.Get(token); ok {
		if _, err := (*conn).Write(data); err != nil {
			(*conn).Close()
			state.PushTunnels.Delete(token)
		}
	} else {
		log.Debug("No such tunnel (PUSH_TUNNEL_DATA): %s", token)
	}
}

func handleDynamicTunnelCreate(c *Client, state *State) {
	port, err := freeport.GetFreePort()
	if err != nil {
		sendMsg(c, message.Message{
			Type: message.DYNAMIC_TUNNEL_CREATE_FAILED,
			Body: message.BodyDynamicTunnelCreateFailed{Reason: err.Error()},
		})
		return
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		sendMsg(c, message.Message{
			Type: message.DYNAMIC_TUNNEL_CREATE_FAILED,
			Body: message.BodyDynamicTunnelCreateFailed{Reason: err.Error()},
		})
		return
	}
	server, err := socks5.New(&socks5.Config{})
	if err != nil {
		sendMsg(c, message.Message{
			Type: message.DYNAMIC_TUNNEL_CREATE_FAILED,
			Body: message.BodyDynamicTunnelCreateFailed{Reason: err.Error()},
		})
		listener.Close()
		return
	}
	state.Socks5Listener = &listener
	go server.Serve(listener)
	log.Success("Socks server started at: %s", addr)
	sendMsg(c, message.Message{
		Type: message.DYNAMIC_TUNNEL_CREATED,
		Body: message.BodyDynamicTunnelCreated{Port: port},
	})
}

func handleDynamicTunnelDestroy(c *Client, state *State) {
	if state.Socks5Listener == nil {
		sendMsg(c, message.Message{
			Type: message.DYNAMIC_TUNNEL_DESTROY_FAILED,
			Body: message.BodyDynamicTunnelDestroyFailed{Reason: "no socks5 server running"},
		})
		return
	}
	err := (*state.Socks5Listener).Close()
	if err != nil {
		log.Error("stopSocks5Server() failed: %s", err.Error())
		sendMsg(c, message.Message{
			Type: message.DYNAMIC_TUNNEL_DESTROY_FAILED,
			Body: message.BodyDynamicTunnelDestroyFailed{Reason: err.Error()},
		})
		return
	}
	state.Socks5Listener = nil
	sendMsg(c, message.Message{
		Type: message.DYNAMIC_TUNNEL_DESTROIED,
		Body: message.BodyDynamicTunnelDestroied{},
	})
}

func handleCallSystem(c *Client, msg *message.Message) {
	token := msg.Body.(*message.BodyCallSystem).Token
	command := msg.Body.(*message.BodyCallSystem).Command
	result, err := exec.Command("sh", "-c", command).Output()
	if err != nil {
		result = []byte("")
	}
	sendMsg(c, message.Message{
		Type: message.CALL_SYSTEM_RESULT,
		Body: message.BodyCallSystemResult{Token: token, Result: result},
	})
}

func handleReadFile(c *Client, msg *message.Message) {
	token := msg.Body.(*message.BodyReadFile).Token
	path := msg.Body.(*message.BodyReadFile).Path
	content, err := os.ReadFile(path)
	if err != nil {
		content = []byte("")
	}
	sendMsg(c, message.Message{
		Type: message.READ_FILE_RESULT,
		Body: message.BodyReadFileResult{Token: token, Result: content},
	})
}

func handleReadFileEx(c *Client, msg *message.Message) {
	token := msg.Body.(*message.BodyReadFileEx).Token
	path := msg.Body.(*message.BodyReadFileEx).Path
	start := msg.Body.(*message.BodyReadFileEx).Start
	size := msg.Body.(*message.BodyReadFileEx).Size

	var result []byte
	f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Error("Failed to open file: %s", err)
		result = []byte{}
	} else {
		buffer := make([]byte, size)
		n, err := f.ReadAt(buffer, start)
		if err != nil {
			log.Info(err.Error())
		}
		log.Info("Reading %d/%d bytes from file %s offset %d", n, size, path, start)
		f.Close()
		result = buffer[0:n]
	}
	sendMsg(c, message.Message{
		Type: message.READ_FILE_EX_RESULT,
		Body: message.BodyReadFileExResult{Token: token, Result: result},
	})
}

func handleFileSize(c *Client, msg *message.Message) {
	token := msg.Body.(*message.BodyFileSize).Token
	path := msg.Body.(*message.BodyFileSize).Path
	fi, err := os.Stat(path)
	var n int64
	if err != nil {
		log.Error(err.Error())
		n = -1
	} else {
		n = fi.Size()
	}
	sendMsg(c, message.Message{
		Type: message.FILE_SIZE_RESULT,
		Body: message.BodyFileSizeResult{Token: token, N: n},
	})
}

func handleWriteFile(c *Client, msg *message.Message) {
	token := msg.Body.(*message.BodyWriteFile).Token
	path := msg.Body.(*message.BodyWriteFile).Path
	content := msg.Body.(*message.BodyWriteFile).Content
	n := -1
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Error("Failed to open file for writing: %s", err)
	} else {
		n, err = f.Write(content)
		if err != nil {
			n = -1
		}
		f.Close()
	}
	sendMsg(c, message.Message{
		Type: message.WRITE_FILE_RESULT,
		Body: message.BodyWriteFileResult{Token: token, N: n},
	})
}

func handleWriteFileEx(c *Client, msg *message.Message) {
	token := msg.Body.(*message.BodyWriteFileEx).Token
	path := msg.Body.(*message.BodyWriteFileEx).Path
	content := msg.Body.(*message.BodyWriteFileEx).Content
	n := -1
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Error("Failed to open file for appending: %s", err)
	} else {
		n, err = f.Write(content)
		if err != nil {
			log.Error(err.Error())
			n = -1
		}
		f.Close()
	}
	sendMsg(c, message.Message{
		Type: message.WRITE_FILE_EX_RESULT,
		Body: message.BodyWriteFileExResult{Token: token, N: n},
	})
}

func handleUpdate(c *Client, msg *message.Message) {
	file, err := os.CreateTemp(os.TempDir(), "temp")
	if err != nil {
		log.Error("Failed to create temp file: %s", err)
		return
	}
	exe := file.Name()
	log.Info("New filename: %s", exe)
	distributorURL := msg.Body.(*message.BodyUpdate).DistributorURL
	version := msg.Body.(*message.BodyUpdate).Version
	log.Info("New version v%s is available, upgrading...", version)
	url := fmt.Sprintf("%s/termite/%s", distributorURL, c.Service)
	if err := selfupdate.UpdateTo(url, exe); err != nil {
		log.Error("Error occurred while updating binary: %s", err)
		return
	}
	log.Info("Update to v%s finished", version)
	log.Info("Restarting...")
	syscall.Exec(exe, []string{exe}, nil)
}
