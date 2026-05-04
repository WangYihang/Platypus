// Package process implements the out-of-process plugin runtime: it
// launches a plugin binary as a child process, dials its gRPC server
// over a Unix socket, and exposes Launch / Health / Call / Stream /
// Stop primitives. The agent's plugin Registry forks here when a
// loaded plugin's manifest declares runtime "process".
//
// Wire contract: see proto/v2/process_plugin.proto. The plugin must:
//   - listen on the Unix socket whose path is in env
//     PLATYPUS_PLUGIN_SOCKET,
//   - print the literal line "READY\n" to stdout once it has bound
//     the listener,
//   - serve v2pb.PluginRuntime,
//   - exit cleanly on Shutdown(); SIGTERM after StopGrace; SIGKILL
//     after a second StopGrace.
//
// Stderr is captured into a bounded ring buffer (StderrTail) for
// post-mortem; nothing is interpreted from it.
package process

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// Config carries everything Launch needs to know. WorkDir is the
// per-plugin state directory the runtime mints — both the plugin's
// listen socket and any plugin-local state files live under it.
// HostSocket is the path of the agent's HostServices listener (the
// plugin will dial it from inside its process).
type Config struct {
	PluginID    string
	Version     string
	BinaryPath  string
	WorkDir     string
	HostSocket  string
	Grants      []string
	ExtraEnv    []string

	// LaunchTimeout caps the total time from exec to a successful
	// Health() call. Default 5s.
	LaunchTimeout time.Duration
	// StopGrace caps each phase of Stop (Shutdown ack, SIGTERM ack).
	// Default 3s.
	StopGrace time.Duration
	// StderrTail caps the ring buffer of captured stderr bytes.
	// Default 8 KiB.
	StderrTail int
}

func (c *Config) defaults() {
	if c.LaunchTimeout == 0 {
		c.LaunchTimeout = 5 * time.Second
	}
	if c.StopGrace == 0 {
		c.StopGrace = 3 * time.Second
	}
	if c.StderrTail == 0 {
		c.StderrTail = 8 * 1024
	}
}

type state int

const (
	stateInitial state = iota
	stateStarting
	stateReady
	stateStopping
	stateDead
)

// Runtime is one running plugin process. It is single-instance: a
// caller wanting two copies of the same plugin constructs two
// Runtimes.
type Runtime struct {
	cfg Config

	cmd        *exec.Cmd
	socketPath string

	stderrBuf  *ringBuffer
	stderrDone chan struct{}

	mu      sync.Mutex
	st      state
	conn    *grpc.ClientConn
	client  v2pb.PluginRuntimeClient
	exitErr error // populated when waiter sees process die
	waited  chan struct{}
}

// New constructs an unstarted Runtime.
func New(cfg Config) *Runtime {
	cfg.defaults()
	return &Runtime{
		cfg:        cfg,
		stderrBuf:  newRingBuffer(cfg.StderrTail),
		stderrDone: make(chan struct{}),
		waited:     make(chan struct{}),
	}
}

// Launch starts the child process, waits for "READY", dials its
// socket, and Health-checks. On any failure the process is reaped
// and the error returned.
func (r *Runtime) Launch(ctx context.Context) error {
	r.mu.Lock()
	if r.st != stateInitial {
		r.mu.Unlock()
		return fmt.Errorf("plugin already launched (state=%d)", r.st)
	}
	r.st = stateStarting
	r.mu.Unlock()

	if err := os.MkdirAll(r.cfg.WorkDir, 0o700); err != nil {
		return fmt.Errorf("workdir: %w", err)
	}
	r.socketPath = filepath.Join(r.cfg.WorkDir, "plugin.sock")
	// Best-effort cleanup of a stale socket from a previous run.
	_ = os.Remove(r.socketPath)

	launchCtx, cancel := context.WithTimeout(ctx, r.cfg.LaunchTimeout)
	defer cancel()

	// Note: exec.Command (NOT CommandContext) — we tie the process
	// lifetime to Stop(), not to the launch deadline. Otherwise the
	// post-Launch cancel() would deliver SIGKILL and kill the plugin
	// just as the agent starts using it.
	cmd := exec.Command(r.cfg.BinaryPath)
	cmd.Dir = r.cfg.WorkDir
	cmd.Env = append(os.Environ(),
		"PLATYPUS_PLUGIN_SOCKET="+r.socketPath,
		"PLATYPUS_HOST_SOCKET="+r.cfg.HostSocket,
		"PLATYPUS_PLUGIN_ID="+r.cfg.PluginID,
		"PLATYPUS_PLUGIN_VERSION="+r.cfg.Version,
		"PLATYPUS_GRANTED_CAPS="+strings.Join(r.cfg.Grants, ","),
		"PLATYPUS_STATE_DIR="+r.cfg.WorkDir,
	)
	cmd.Env = append(cmd.Env, r.cfg.ExtraEnv...)
	// Detach from parent process group so a Ctrl-C delivered to the
	// agent's tty does NOT race-deliver SIGINT to the plugin too:
	// we want plugin shutdown to go through Stop().
	cmd.SysProcAttr = sysProcAttr()

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %q: %w", r.cfg.BinaryPath, err)
	}
	r.cmd = cmd

	go r.drainStderr(stderrPipe)
	go r.waitForExit()

	if err := r.awaitReady(launchCtx, stdoutPipe); err != nil {
		r.killAndReap()
		return err
	}

	if err := r.dial(launchCtx); err != nil {
		r.killAndReap()
		return fmt.Errorf("dial plugin socket: %w", err)
	}

	if err := r.handshakeHealth(launchCtx); err != nil {
		r.killAndReap()
		return err
	}

	r.mu.Lock()
	r.st = stateReady
	r.mu.Unlock()
	return nil
}

// awaitReady reads stdout line-by-line until it sees "READY" or the
// context expires. Any other lines are forwarded to the stderr ring
// buffer (so plugin authors can debug bring-up problems without
// having to refactor their logging).
func (r *Runtime) awaitReady(ctx context.Context, rdr io.Reader) error {
	type lineRes struct {
		s   string
		err error
	}
	ch := make(chan lineRes, 1)
	go func() {
		br := bufio.NewReader(rdr)
		for {
			line, err := br.ReadString('\n')
			if line != "" {
				trimmed := strings.TrimRight(line, "\r\n")
				if trimmed == "READY" {
					ch <- lineRes{s: trimmed}
					return
				}
				r.stderrBuf.WriteString("[stdout] " + trimmed + "\n")
			}
			if err != nil {
				ch <- lineRes{err: err}
				return
			}
		}
	}()
	select {
	case res := <-ch:
		if res.err != nil {
			return fmt.Errorf("plugin exited before READY: %w (stderr: %s)", res.err, r.StderrTail())
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("plugin did not emit READY within %s (stderr: %s)", r.cfg.LaunchTimeout, r.StderrTail())
	}
}

// dial opens the gRPC connection to the plugin's Unix socket. It
// retries until ctx expires; the plugin may have printed READY
// microseconds before binding the listener.
func (r *Runtime) dial(ctx context.Context) error {
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return lastErr
			}
			return ctx.Err()
		default:
		}
		// Wait for the socket file to appear before dialing — the
		// plugin may print READY a hair before bind() returns.
		if _, err := os.Stat(r.socketPath); err != nil {
			lastErr = err
			time.Sleep(20 * time.Millisecond)
			continue
		}
		conn, err := grpc.NewClient(
			"unix:"+r.socketPath,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			lastErr = err
			time.Sleep(20 * time.Millisecond)
			continue
		}
		r.mu.Lock()
		r.conn = conn
		r.client = v2pb.NewPluginRuntimeClient(conn)
		r.mu.Unlock()
		return nil
	}
}

// handshakeHealth retries Health() until ready=true or ctx expires.
// Verifies the plugin self-identifies as the expected id+version.
func (r *Runtime) handshakeHealth(ctx context.Context) error {
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("health: %w", lastErr)
			}
			return ctx.Err()
		default:
		}
		hctx, cancel := context.WithTimeout(ctx, time.Second)
		resp, err := r.client.Health(hctx, &emptypb.Empty{})
		cancel()
		if err != nil {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if resp.PluginId != r.cfg.PluginID {
			return fmt.Errorf("plugin identifies as %q, expected %q", resp.PluginId, r.cfg.PluginID)
		}
		if resp.Version != r.cfg.Version {
			return fmt.Errorf("plugin reports version %q, expected %q", resp.Version, r.cfg.Version)
		}
		if resp.Ready {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Health calls Health() once, with no retries. Used by liveness
// probes after Launch has succeeded.
func (r *Runtime) Health(ctx context.Context) (*v2pb.PluginHealthResponse, error) {
	r.mu.Lock()
	cli := r.client
	r.mu.Unlock()
	if cli == nil {
		return nil, errors.New("plugin not launched")
	}
	return cli.Health(ctx, &emptypb.Empty{})
}

// Stop shuts down the plugin: gRPC Shutdown(), then SIGTERM after
// StopGrace, then SIGKILL after another StopGrace. Returns nil if
// the process exited cleanly via Shutdown(); otherwise the
// underlying signal/exit reason.
func (r *Runtime) Stop(ctx context.Context) error {
	r.mu.Lock()
	if r.st == stateDead {
		r.mu.Unlock()
		return nil
	}
	if r.st == stateInitial {
		r.mu.Unlock()
		return errors.New("plugin not launched")
	}
	r.st = stateStopping
	cli := r.client
	conn := r.conn
	r.mu.Unlock()

	if cli != nil {
		sctx, cancel := context.WithTimeout(ctx, r.cfg.StopGrace)
		_, _ = cli.Shutdown(sctx, &emptypb.Empty{})
		cancel()
	}
	if conn != nil {
		_ = conn.Close()
	}

	if r.waitFor(r.cfg.StopGrace) {
		return r.exitErr
	}

	// Process still alive: escalate to SIGTERM.
	if r.cmd != nil && r.cmd.Process != nil {
		_ = r.cmd.Process.Signal(syscall.SIGTERM)
	}
	if r.waitFor(r.cfg.StopGrace) {
		return r.exitErr
	}

	// Last resort.
	if r.cmd != nil && r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
	}
	r.waitFor(2 * time.Second)
	return r.exitErr
}

// waitFor blocks up to d for the child's exit. Returns true if it
// observed the exit.
func (r *Runtime) waitFor(d time.Duration) bool {
	select {
	case <-r.waited:
		return true
	case <-time.After(d):
		return false
	}
}

// killAndReap is the launch-failure cleanup path. Best-effort.
func (r *Runtime) killAndReap() {
	if r.conn != nil {
		_ = r.conn.Close()
	}
	if r.cmd != nil && r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
	}
	r.waitFor(2 * time.Second)
}

func (r *Runtime) waitForExit() {
	defer close(r.waited)
	err := r.cmd.Wait()
	r.mu.Lock()
	r.exitErr = err
	r.st = stateDead
	r.mu.Unlock()
	// Wait for stderr drain to flush so StderrTail() is complete.
	<-r.stderrDone
}

func (r *Runtime) drainStderr(rdr io.Reader) {
	defer close(r.stderrDone)
	buf := make([]byte, 4*1024)
	for {
		n, err := rdr.Read(buf)
		if n > 0 {
			r.stderrBuf.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// StderrTail returns the captured-stderr ring buffer's contents.
// Safe to call at any state; produces an empty string before the
// first stderr byte arrives.
func (r *Runtime) StderrTail() string {
	return r.stderrBuf.String()
}

// Wait blocks until the child process exits. After Stop or a crash
// the returned error is the exec.Cmd.Wait() error (nil for exit 0,
// *exec.ExitError otherwise).
func (r *Runtime) Wait() error {
	<-r.waited
	return r.exitErr
}
