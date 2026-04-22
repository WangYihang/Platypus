package agent

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/phayes/freeport"
	socks5 "github.com/things-go/go-socks5"

	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/utils/str"
	"github.com/WangYihang/Platypus/internal/utils/update"
	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

const protocolVersion = 1

// HandleConnection runs the main message dispatch loop for the agent.
func HandleConnection(c *Client, state *State) {
	for {
		env, err := c.RecvEnvelope()
		if err != nil {
			log.Error("Network error: %s", err)
			break
		}
		dispatchEnvelope(c, state, env)
	}
}

// dispatchEnvelope routes a single envelope to the appropriate handler.
// Shared between the TLS loop (HandleConnection) and mesh delivery
// (HandleMeshEnvelope).
func dispatchEnvelope(c *Client, state *State, env *agentpb.Envelope) {
	switch p := env.Payload.(type) {
	case *agentpb.Envelope_Stdio:
		if proc, ok := state.Processes.Get(p.Stdio.Key); ok {
			proc.Ptmx.Write(p.Stdio.Data)
		}
	case *agentpb.Envelope_WindowSize:
		ws := &pty.Winsize{
			Cols: uint16(p.WindowSize.Columns),
			Rows: uint16(p.WindowSize.Rows),
		}
		if proc, ok := state.Processes.Get(p.WindowSize.Key); ok {
			pty.Setsize(proc.Ptmx, ws)
		}
	case *agentpb.Envelope_ProcessStartRequest:
		handleProcessStart(c, state, env.RequestId, p.ProcessStartRequest)
	case *agentpb.Envelope_ProcessStartedResponse:
	case *agentpb.Envelope_ProcessStopped:
	case *agentpb.Envelope_GetClientInfoRequest:
		handleGetClientInfo(c, env.RequestId)
	case *agentpb.Envelope_DuplicateClient:
		log.Error("Duplicated connection")
		os.Exit(0)
	case *agentpb.Envelope_ProcessTerminateRequest:
		handleProcessTerminate(state, p.ProcessTerminateRequest)
	case *agentpb.Envelope_TunnelConnectRequest:
		handleTunnelConnect(c, state, p.TunnelConnectRequest)
	case *agentpb.Envelope_TunnelData:
		handleTunnelData(state, p.TunnelData)
	case *agentpb.Envelope_TunnelCloseRequest:
		handleTunnelClose(c, state, p.TunnelCloseRequest)
	case *agentpb.Envelope_TunnelCreateRequest:
		handleTunnelCreate(c, state, p.TunnelCreateRequest)
	case *agentpb.Envelope_TunnelConnectedResponse:
		handleTunnelConnected(c, state, p.TunnelConnectedResponse)
	case *agentpb.Envelope_TunnelDisconnected:
		if conn, ok := state.PushTunnels.GetAndDelete(p.TunnelDisconnected.TunnelId); ok {
			(*conn).Close()
		}
	case *agentpb.Envelope_TunnelConnectFailed:
		if conn, ok := state.PushTunnels.GetAndDelete(p.TunnelConnectFailed.TunnelId); ok {
			(*conn).Close()
		}
	case *agentpb.Envelope_Socks5CreateRequest:
		handleSocks5Create(c, state)
	case *agentpb.Envelope_Socks5DestroyRequest:
		handleSocks5Destroy(c, state)
	case *agentpb.Envelope_ExecRequest:
		handleExec(c, env.RequestId, p.ExecRequest)
	case *agentpb.Envelope_ReadFileRequest:
		handleReadFile(c, env.RequestId, p.ReadFileRequest)
	case *agentpb.Envelope_FileSizeRequest:
		handleFileSize(c, env.RequestId, p.FileSizeRequest)
	case *agentpb.Envelope_WriteFileRequest:
		handleWriteFile(c, env.RequestId, p.WriteFileRequest)
	case *agentpb.Envelope_ListDirRequest:
		handleListDir(c, env.RequestId, p.ListDirRequest)
	case *agentpb.Envelope_StatRequest:
		handleStat(c, env.RequestId, p.StatRequest)
	case *agentpb.Envelope_DeleteRequest:
		handleDelete(c, env.RequestId, p.DeleteRequest)
	case *agentpb.Envelope_RenameRequest:
		handleRename(c, env.RequestId, p.RenameRequest)
	case *agentpb.Envelope_MkdirRequest:
		handleMkdir(c, env.RequestId, p.MkdirRequest)
	case *agentpb.Envelope_ChmodRequest:
		handleChmod(c, env.RequestId, p.ChmodRequest)
	case *agentpb.Envelope_UpdateRequest:
		handleUpdate(c, p.UpdateRequest)
	case *agentpb.Envelope_SessionRenewResponse:
		// Route back to whichever goroutine sent the matching
		// SessionRenewRequest. The renewal loop (see StartRenewalLoop)
		// registers a channel keyed by the request_id before sending;
		// here we just forward.
		deliverRenewalResponse(env.RequestId, p.SessionRenewResponse)
	}
}

func send(c *Client, env *agentpb.Envelope) {
	env.Version = protocolVersion
	env.Timestamp = time.Now().UnixNano()
	if err := c.SendEnvelope(env); err != nil {
		log.Error("Send error: %s", err)
	}
}

func handleProcessStart(c *Client, state *State, reqID string, req *agentpb.ProcessStartRequest) {
	if req.Path == "" {
		return
	}
	log.Success("Starting process: %s", req.Path)
	process := exec.Command(req.Path)
	process.Env = os.Environ()
	process.Env = append(process.Env, "HISTORY=", "HISTSIZE=0", "HISTSAVE=",
		"HISTZONE=", "HISTLOG=", "HISTFILE=", "HISTFILE=/dev/null", "HISTFILESIZE=0")

	ws := pty.Winsize{Rows: uint16(req.WindowRows), Cols: uint16(req.WindowColumns)}
	ptmx, _ := pty.StartWithSize(process, &ws)
	state.Processes.Set(req.Key, &AgentProcess{WindowSize: &ws, Ptmx: ptmx, Process: process})
	log.Success("Process started: %d", process.Process.Pid)
	defer func() { _ = ptmx.Close() }()

	send(c, &agentpb.Envelope{
		RequestId: reqID,
		Payload: &agentpb.Envelope_ProcessStartedResponse{
			ProcessStartedResponse: &agentpb.ProcessStartedResponse{Key: req.Key, Pid: int32(process.Process.Pid)},
		},
	})

	go func() {
		for {
			buffer := make([]byte, 0x4000)
			n, err := ptmx.Read(buffer)
			if err != nil {
				if err == io.EOF {
					send(c, &agentpb.Envelope{
						Payload: &agentpb.Envelope_ProcessStopped{
							ProcessStopped: &agentpb.ProcessStoppedNotice{Key: req.Key, ExitCode: 0},
						},
					})
				}
				break
			}
			if n > 0 {
				send(c, &agentpb.Envelope{
					Payload: &agentpb.Envelope_Stdio{
						Stdio: &agentpb.StdioData{Key: req.Key, Data: buffer[0:n]},
					},
				})
			}
		}
	}()

	go func() {
		err := process.Wait()
		exitCode := int32(0)
		if err != nil {
			exitCode = int32(err.(*exec.ExitError).ExitCode())
		}
		fmt.Println("Exit code: ", exitCode)
		send(c, &agentpb.Envelope{
			Payload: &agentpb.Envelope_ProcessStopped{
				ProcessStopped: &agentpb.ProcessStoppedNotice{Key: req.Key, ExitCode: exitCode},
			},
		})
		state.Processes.Delete(req.Key)
	}()
}

func handleGetClientInfo(c *Client, reqID string) {
	userInfo, err := user.Current()
	username := "Unknown"
	if err == nil {
		username = userInfo.Username
	}
	hostname, _ := os.Hostname()

	var languages []string
	if p, err := exec.LookPath("python2"); err == nil && p != "" {
		languages = append(languages, "python2")
	}
	if p, err := exec.LookPath("python3"); err == nil && p != "" {
		languages = append(languages, "python3")
	}
	if p, err := exec.LookPath("perl"); err == nil && p != "" {
		languages = append(languages, "perl")
	}

	interfaces := map[string]string{}
	ifaces, _ := net.Interfaces()
	for _, i := range ifaces {
		interfaces[i.Name] = i.HardwareAddr.String()
	}

	send(c, &agentpb.Envelope{
		RequestId: reqID,
		Payload: &agentpb.Envelope_ClientInfoResponse{
			ClientInfoResponse: &agentpb.ClientInfoResponse{
				Version:            update.Version,
				Os:                 runtime.GOOS,
				Arch:               runtime.GOARCH,
				User:               username,
				Hostname:           hostname,
				NetworkInterfaces:  interfaces,
				AvailableLanguages: languages,
				MachineId:          MachineID(),
			},
		},
	})
}

func handleProcessTerminate(state *State, req *agentpb.ProcessTerminateRequest) {
	log.Success("Request terminate %s", req.Key)
	if p, ok := state.Processes.Get(req.Key); ok {
		if proc, err := os.FindProcess(p.Process.Process.Pid); err != nil {
			log.Error("Unable to find process: %s", err)
		} else {
			proc.Signal(syscall.SIGTERM)
			p.Ptmx.Close()
		}
	}
}

func handleTunnelConnect(c *Client, state *State, req *agentpb.TunnelConnectRequest) {
	conn, err := net.Dial("tcp", req.Address)
	if err != nil {
		log.Error("%s", err.Error())
		send(c, &agentpb.Envelope{
			Payload: &agentpb.Envelope_TunnelConnectFailed{
				TunnelConnectFailed: &agentpb.TunnelConnectFailed{TunnelId: req.TunnelId, Reason: err.Error()},
			},
		})
		return
	}

	send(c, &agentpb.Envelope{
		Payload: &agentpb.Envelope_TunnelConnectedResponse{
			TunnelConnectedResponse: &agentpb.TunnelConnectedResponse{TunnelId: req.TunnelId},
		},
	})

	state.PullTunnels.Set(req.TunnelId, &conn)
	go func() {
		for {
			buffer := make([]byte, 0x100)
			n, err := conn.Read(buffer)
			if err != nil {
				log.Success("Tunnel (%s) disconnected: %s", req.TunnelId, err.Error())
				send(c, &agentpb.Envelope{
					Payload: &agentpb.Envelope_TunnelDisconnected{
						TunnelDisconnected: &agentpb.TunnelDisconnectedNotice{TunnelId: req.TunnelId},
					},
				})
				conn.Close()
				break
			}
			if n > 0 {
				send(c, &agentpb.Envelope{
					Payload: &agentpb.Envelope_TunnelData{
						TunnelData: &agentpb.TunnelData{TunnelId: req.TunnelId, Data: buffer[0:n]},
					},
				})
			}
		}
	}()
}

func handleTunnelData(state *State, td *agentpb.TunnelData) {
	if conn, ok := state.PullTunnels.Get(td.TunnelId); ok {
		if _, err := (*conn).Write(td.Data); err != nil {
			(*conn).Close()
		}
	} else {
		log.Debug("No such tunnel: %s", td.TunnelId)
	}
}

func handleTunnelClose(c *Client, state *State, req *agentpb.TunnelCloseRequest) {
	if conn, ok := state.PullTunnels.GetAndDelete(req.TunnelId); ok {
		log.Info("Closing connection: %s", (*conn).RemoteAddr().String())
		(*conn).Close()
		send(c, &agentpb.Envelope{
			Payload: &agentpb.Envelope_TunnelClosed{
				TunnelClosed: &agentpb.TunnelClosedNotice{TunnelId: req.TunnelId},
			},
		})
	} else {
		log.Debug("No such tunnel: %s", req.TunnelId)
	}
}

func handleTunnelCreate(c *Client, state *State, req *agentpb.TunnelCreateRequest) {
	log.Info("Creating remote port forwarding from %s", req.Address)
	server, err := net.Listen("tcp", req.Address)
	if err != nil {
		log.Error("Server (%s) create failed: %s", req.Address, err.Error())
		send(c, &agentpb.Envelope{
			Payload: &agentpb.Envelope_TunnelCreateFailed{
				TunnelCreateFailed: &agentpb.TunnelCreateFailed{Address: req.Address, Reason: err.Error()},
			},
		})
		return
	}
	log.Success("Server created (%s)", req.Address)
	send(c, &agentpb.Envelope{
		Payload: &agentpb.Envelope_TunnelCreatedResponse{
			TunnelCreatedResponse: &agentpb.TunnelCreatedResponse{Address: req.Address},
		},
	})
	go func() {
		for {
			conn, err := server.Accept()
			if err != nil {
				break
			}
			tunnelID := str.RandomString(0x10)
			log.Success("Connection came from: %s", conn.RemoteAddr().String())
			send(c, &agentpb.Envelope{
				Payload: &agentpb.Envelope_TunnelConnectRequest{
					TunnelConnectRequest: &agentpb.TunnelConnectRequest{TunnelId: tunnelID, Address: req.Address},
				},
			})
			state.PushTunnels.Set(tunnelID, &conn)
		}
	}()
}

func handleTunnelConnected(c *Client, state *State, resp *agentpb.TunnelConnectedResponse) {
	log.Success("Connection (%s) connected", resp.TunnelId)
	conn, ok := state.PushTunnels.Get(resp.TunnelId)
	if !ok {
		log.Debug("No such tunnel: %s", resp.TunnelId)
		return
	}
	go func() {
		for {
			buffer := make([]byte, 0x4000)
			n, err := (*conn).Read(buffer)
			if err != nil {
				log.Error("Read from (%s) failed: %s", resp.TunnelId, err.Error())
				send(c, &agentpb.Envelope{
					Payload: &agentpb.Envelope_TunnelDisconnected{
						TunnelDisconnected: &agentpb.TunnelDisconnectedNotice{TunnelId: resp.TunnelId, Reason: err.Error()},
					},
				})
				(*conn).Close()
				state.PushTunnels.Delete(resp.TunnelId)
				break
			}
			log.Debug("%d bytes read from (%s)", n, resp.TunnelId)
			send(c, &agentpb.Envelope{
				Payload: &agentpb.Envelope_TunnelData{
					TunnelData: &agentpb.TunnelData{TunnelId: resp.TunnelId, Data: buffer[0:n]},
				},
			})
		}
	}()
}

func handleSocks5Create(c *Client, state *State) {
	port, err := freeport.GetFreePort()
	if err != nil {
		send(c, &agentpb.Envelope{
			Payload: &agentpb.Envelope_Socks5CreateFailed{
				Socks5CreateFailed: &agentpb.Socks5CreateFailed{Reason: err.Error()},
			},
		})
		return
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		send(c, &agentpb.Envelope{
			Payload: &agentpb.Envelope_Socks5CreateFailed{
				Socks5CreateFailed: &agentpb.Socks5CreateFailed{Reason: err.Error()},
			},
		})
		return
	}
	srv := socks5.NewServer()
	state.Socks5Listener = &listener
	go srv.Serve(listener)
	log.Success("Socks server started at: %s", addr)
	send(c, &agentpb.Envelope{
		Payload: &agentpb.Envelope_Socks5CreatedResponse{
			Socks5CreatedResponse: &agentpb.Socks5CreatedResponse{Port: int32(port)},
		},
	})
}

func handleSocks5Destroy(c *Client, state *State) {
	if state.Socks5Listener == nil {
		send(c, &agentpb.Envelope{
			Payload: &agentpb.Envelope_Socks5DestroyFailed{
				Socks5DestroyFailed: &agentpb.Socks5DestroyFailed{Reason: "no socks5 server running"},
			},
		})
		return
	}
	err := (*state.Socks5Listener).Close()
	if err != nil {
		send(c, &agentpb.Envelope{
			Payload: &agentpb.Envelope_Socks5DestroyFailed{
				Socks5DestroyFailed: &agentpb.Socks5DestroyFailed{Reason: err.Error()},
			},
		})
		return
	}
	state.Socks5Listener = nil
	send(c, &agentpb.Envelope{
		Payload: &agentpb.Envelope_Socks5DestroyedResponse{
			Socks5DestroyedResponse: &agentpb.Socks5DestroyedResponse{},
		},
	})
}

func handleExec(c *Client, reqID string, req *agentpb.ExecRequest) {
	result, err := exec.Command("sh", "-c", req.Command).Output()
	errMsg := ""
	exitCode := int32(0)
	if err != nil {
		errMsg = err.Error()
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = int32(exitErr.ExitCode())
		}
		if result == nil {
			result = []byte{}
		}
	}
	send(c, &agentpb.Envelope{
		RequestId: reqID,
		Payload: &agentpb.Envelope_ExecResponse{
			ExecResponse: &agentpb.ExecResponse{Output: result, ExitCode: exitCode, Error: errMsg},
		},
	})
}

func handleReadFile(c *Client, reqID string, req *agentpb.ReadFileRequest) {
	var data []byte
	var errMsg string

	if req.Size == 0 {
		// Read entire file
		content, err := os.ReadFile(req.Path)
		if err != nil {
			errMsg = err.Error()
			data = []byte{}
		} else {
			data = content
		}
	} else {
		f, err := os.OpenFile(req.Path, os.O_RDONLY, 0644)
		if err != nil {
			errMsg = err.Error()
			data = []byte{}
		} else {
			buffer := make([]byte, req.Size)
			n, err := f.ReadAt(buffer, req.Offset)
			if err != nil && err != io.EOF {
				errMsg = err.Error()
			}
			f.Close()
			data = buffer[0:n]
		}
	}

	send(c, &agentpb.Envelope{
		RequestId: reqID,
		Payload: &agentpb.Envelope_ReadFileResponse{
			ReadFileResponse: &agentpb.ReadFileResponse{Data: data, Error: errMsg},
		},
	})
}

func handleFileSize(c *Client, reqID string, req *agentpb.FileSizeRequest) {
	fi, err := os.Stat(req.Path)
	var size int64
	var errMsg string
	if err != nil {
		size = -1
		errMsg = err.Error()
	} else {
		size = fi.Size()
	}
	send(c, &agentpb.Envelope{
		RequestId: reqID,
		Payload: &agentpb.Envelope_FileSizeResponse{
			FileSizeResponse: &agentpb.FileSizeResponse{Size: size, Error: errMsg},
		},
	})
}

func handleWriteFile(c *Client, reqID string, req *agentpb.WriteFileRequest) {
	flags := os.O_WRONLY | os.O_CREATE
	if req.Append {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}

	var bytesWritten int64
	var errMsg string
	f, err := os.OpenFile(req.Path, flags, 0644)
	if err != nil {
		errMsg = err.Error()
	} else {
		n, err := f.Write(req.Data)
		if err != nil {
			errMsg = err.Error()
		}
		bytesWritten = int64(n)
		f.Close()
	}

	send(c, &agentpb.Envelope{
		RequestId: reqID,
		Payload: &agentpb.Envelope_WriteFileResponse{
			WriteFileResponse: &agentpb.WriteFileResponse{BytesWritten: bytesWritten, Error: errMsg},
		},
	})
}

// maxListDirEntries caps how many entries a single ListDir response may
// carry. A FileEntry is ~100 bytes on the wire; 5000 × 100 = 500 KiB,
// well under the 16 MiB envelope limit even with long names.
const maxListDirEntries = 5000

// entryFromLstat populates a FileEntry by lstat'ing a path. Symlinks
// are not traversed — if the target is itself a symlink to something
// else, the UI decides whether to follow.
func entryFromLstat(parent, name string) *agentpb.FileEntry {
	entry := &agentpb.FileEntry{Name: name}
	full := filepath.Join(parent, name)
	fi, err := os.Lstat(full)
	if err != nil {
		entry.Error = err.Error()
		return entry
	}
	mode := fi.Mode()
	entry.Size = fi.Size()
	entry.Mode = uint32(mode)
	entry.ModTimeUnix = fi.ModTime().UnixNano()
	entry.IsDir = mode.IsDir()
	if mode&os.ModeSymlink != 0 {
		entry.IsSymlink = true
		if target, rlErr := os.Readlink(full); rlErr == nil {
			entry.SymlinkTarget = target
		}
	}
	return entry
}

func handleListDir(c *Client, reqID string, req *agentpb.ListDirRequest) {
	resp := &agentpb.ListDirResponse{}
	if req.Path == "" {
		resp.Error = "path is empty"
		send(c, &agentpb.Envelope{RequestId: reqID, Payload: &agentpb.Envelope_ListDirResponse{ListDirResponse: resp}})
		return
	}
	raw, err := os.ReadDir(req.Path)
	if err != nil {
		resp.Error = err.Error()
		send(c, &agentpb.Envelope{RequestId: reqID, Payload: &agentpb.Envelope_ListDirResponse{ListDirResponse: resp}})
		return
	}
	// Stable alphabetical ordering so offset/limit paging is deterministic.
	sort.Slice(raw, func(i, j int) bool { return raw[i].Name() < raw[j].Name() })

	total := int64(len(raw))
	resp.Total = total

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}

	limit := req.Limit
	if limit <= 0 || limit > maxListDirEntries {
		limit = maxListDirEntries
	}

	end := offset + limit
	if end > total {
		end = total
	}
	resp.Eof = end >= total

	for i := offset; i < end; i++ {
		resp.Entries = append(resp.Entries, entryFromLstat(req.Path, raw[i].Name()))
	}

	send(c, &agentpb.Envelope{RequestId: reqID, Payload: &agentpb.Envelope_ListDirResponse{ListDirResponse: resp}})
}

func handleStat(c *Client, reqID string, req *agentpb.StatRequest) {
	resp := &agentpb.StatResponse{}
	if req.Path == "" {
		resp.Error = "path is empty"
		send(c, &agentpb.Envelope{RequestId: reqID, Payload: &agentpb.Envelope_StatResponse{StatResponse: resp}})
		return
	}
	resp.Entry = entryFromLstat(filepath.Dir(req.Path), filepath.Base(req.Path))
	if resp.Entry.Error != "" {
		resp.Error = resp.Entry.Error
	}
	send(c, &agentpb.Envelope{RequestId: reqID, Payload: &agentpb.Envelope_StatResponse{StatResponse: resp}})
}

func handleDelete(c *Client, reqID string, req *agentpb.DeleteRequest) {
	resp := &agentpb.DeleteResponse{}
	var err error
	if req.Path == "" {
		resp.Error = "path is empty"
	} else if req.Recursive {
		err = os.RemoveAll(req.Path)
	} else {
		err = os.Remove(req.Path)
	}
	if err != nil {
		resp.Error = err.Error()
	}
	send(c, &agentpb.Envelope{RequestId: reqID, Payload: &agentpb.Envelope_DeleteResponse{DeleteResponse: resp}})
}

func handleRename(c *Client, reqID string, req *agentpb.RenameRequest) {
	resp := &agentpb.RenameResponse{}
	if req.From == "" || req.To == "" {
		resp.Error = "from and to are required"
	} else if err := os.Rename(req.From, req.To); err != nil {
		// Cross-filesystem renames return EXDEV on Linux. Surface the
		// raw error verbatim so the UI can tell users to copy+delete.
		resp.Error = err.Error()
	}
	send(c, &agentpb.Envelope{RequestId: reqID, Payload: &agentpb.Envelope_RenameResponse{RenameResponse: resp}})
}

func handleMkdir(c *Client, reqID string, req *agentpb.MkdirRequest) {
	resp := &agentpb.MkdirResponse{}
	mode := os.FileMode(req.Mode & 0o7777)
	if mode == 0 {
		mode = 0o755
	}
	var err error
	if req.Path == "" {
		resp.Error = "path is empty"
	} else if req.Parents {
		err = os.MkdirAll(req.Path, mode)
	} else {
		err = os.Mkdir(req.Path, mode)
	}
	if err != nil {
		resp.Error = err.Error()
	}
	send(c, &agentpb.Envelope{RequestId: reqID, Payload: &agentpb.Envelope_MkdirResponse{MkdirResponse: resp}})
}

func handleChmod(c *Client, reqID string, req *agentpb.ChmodRequest) {
	resp := &agentpb.ChmodResponse{}
	// On Windows, os.Chmod only honours the owner-write bit; that's a
	// platform limitation we surface as-is.
	if req.Path == "" {
		resp.Error = "path is empty"
	} else if err := os.Chmod(req.Path, os.FileMode(req.Mode&0o7777)); err != nil {
		resp.Error = err.Error()
	}
	send(c, &agentpb.Envelope{RequestId: reqID, Payload: &agentpb.Envelope_ChmodResponse{ChmodResponse: resp}})
}
