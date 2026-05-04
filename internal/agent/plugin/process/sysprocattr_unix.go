//go:build !windows

package process

import "syscall"

// sysProcAttr puts the child in its own process group on Unix. This
// keeps a Ctrl-C delivered to the agent's controlling tty from
// race-delivering SIGINT to plugins; we want plugin shutdown to
// always go through the gRPC Shutdown() path.
func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
