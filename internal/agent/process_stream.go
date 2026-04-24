package agent

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"

	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// HandleProcessStream is the agent-side handler for a
// STREAM_TYPE_PROCESS_OPEN stream. Responsibilities:
//
//  1. Spawn the child named by req (PTY-backed when req.Pty, else
//     a plain exec with stdin/stdout/stderr pipes).
//  2. Write a ProcessOpenResponse as the first frame on the
//     stream — populated with Pid on success, Error on spawn
//     failure (stream then closes).
//  3. Pump bytes both directions until either the child exits or
//     the stream closes:
//     server -> agent: ProcessFrame.stdin → child stdin
//     ProcessFrame.resize → PTY window resize
//     agent -> server: child stdout → ProcessFrame.stdout
//     child stderr → ProcessFrame.stderr (non-PTY only)
//  4. Emit a ProcessFrame.exit frame with the child's status.
//  5. Close the stream.
//
// Returns a non-nil error only on an unexpected framing / I/O
// failure; a child that exits with non-zero status is a normal
// outcome and surfaces via the ExitInfo frame, not a returned err.
func HandleProcessStream(ctx context.Context, stream io.ReadWriteCloser, req *v2pb.ProcessOpenRequest) error {
	defer func() { _ = stream.Close() }()
	if req == nil {
		return writeOpenError(stream, "nil ProcessOpenRequest")
	}
	if req.Command == "" {
		return writeOpenError(stream, "empty command")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(ctx, req.Command, req.Args...)
	if req.Cwd != "" {
		cmd.Dir = req.Cwd
	}
	if len(req.Env) > 0 {
		cmd.Env = append(os.Environ(), flattenEnv(req.Env)...)
	}

	if req.Pty {
		return runPTY(ctx, stream, cmd, req)
	}
	return runPipes(ctx, stream, cmd)
}

// runPTY spawns cmd under a PTY using creack/pty, reports the pid,
// and runs the bidirectional pump.
func runPTY(ctx context.Context, stream io.ReadWriteCloser, cmd *exec.Cmd, req *v2pb.ProcessOpenRequest) error {
	winsize := &pty.Winsize{}
	if req.Cols > 0 {
		winsize.Cols = uint16(req.Cols)
	}
	if req.Rows > 0 {
		winsize.Rows = uint16(req.Rows)
	}
	ptmx, err := pty.StartWithSize(cmd, winsize)
	if err != nil {
		return writeOpenError(stream, "spawn: "+err.Error())
	}
	defer func() { _ = ptmx.Close() }()

	if err := link.WriteFrame(stream, &v2pb.ProcessOpenResponse{Pid: int64(cmd.Process.Pid)}); err != nil {
		_ = killCmd(cmd)
		return err
	}

	return pumpPTY(ctx, stream, ptmx, cmd)
}

// runPipes is the non-PTY path: three pipes (stdin/stdout/stderr)
// and exec.Cmd.Start.
func runPipes(ctx context.Context, stream io.ReadWriteCloser, cmd *exec.Cmd) error {
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return writeOpenError(stream, "stdin pipe: "+err.Error())
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return writeOpenError(stream, "stdout pipe: "+err.Error())
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return writeOpenError(stream, "stderr pipe: "+err.Error())
	}
	if err := cmd.Start(); err != nil {
		return writeOpenError(stream, "start: "+err.Error())
	}

	if err := link.WriteFrame(stream, &v2pb.ProcessOpenResponse{Pid: int64(cmd.Process.Pid)}); err != nil {
		_ = killCmd(cmd)
		return err
	}

	return pumpPipes(ctx, stream, cmd, stdinPipe, stdoutPipe, stderrPipe)
}

// pumpPTY runs the three coroutines for the PTY path:
//   - server -> PTY (stdin + resize frames)
//   - PTY -> server (stdout frames)
//   - child wait (→ exit frame)
func pumpPTY(ctx context.Context, stream io.ReadWriteCloser, ptmx *os.File, cmd *exec.Cmd) error {
	// writeMu serialises ProcessFrame writes on the single stream.
	var writeMu sync.Mutex
	write := func(f *v2pb.ProcessFrame) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return link.WriteFrame(stream, f)
	}

	// server -> PTY
	go func() {
		for {
			var f v2pb.ProcessFrame
			if err := link.ReadFrame(stream, &f); err != nil {
				_ = killCmd(cmd)
				return
			}
			switch p := f.Payload.(type) {
			case *v2pb.ProcessFrame_Stdin:
				if _, err := ptmx.Write(p.Stdin); err != nil {
					return
				}
			case *v2pb.ProcessFrame_Resize:
				_ = pty.Setsize(ptmx, &pty.Winsize{
					Cols: uint16(p.Resize.Cols),
					Rows: uint16(p.Resize.Rows),
				})
			}
		}
	}()

	// PTY -> server
	ptyDone := make(chan struct{})
	go func() {
		defer close(ptyDone)
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				if wErr := write(&v2pb.ProcessFrame{
					Payload: &v2pb.ProcessFrame_Stdout{Stdout: append([]byte(nil), buf[:n]...)},
				}); wErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Wait for child to exit.
	waitErr := cmd.Wait()
	// Give the PTY reader a brief moment to drain any trailing
	// bytes the kernel still has buffered (common on small outputs).
	select {
	case <-ptyDone:
	case <-ctx.Done():
	}

	_ = write(&v2pb.ProcessFrame{Payload: &v2pb.ProcessFrame_Exit{Exit: waitInfo(cmd, waitErr)}})
	return nil
}

// pumpPipes is the non-PTY equivalent. stdout/stderr are separate
// ReadCloser pipes so both feed back to the server as distinct
// frame types.
func pumpPipes(ctx context.Context, stream io.ReadWriteCloser, cmd *exec.Cmd, stdin io.WriteCloser, stdoutP, stderrP io.ReadCloser) error {
	var writeMu sync.Mutex
	write := func(f *v2pb.ProcessFrame) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return link.WriteFrame(stream, f)
	}

	// server -> agent: stdin frames only (resize is meaningless without a PTY).
	go func() {
		defer func() { _ = stdin.Close() }()
		for {
			var f v2pb.ProcessFrame
			if err := link.ReadFrame(stream, &f); err != nil {
				_ = killCmd(cmd)
				return
			}
			if s := f.GetStdin(); s != nil {
				if _, err := stdin.Write(s); err != nil {
					return
				}
			}
		}
	}()

	var wg sync.WaitGroup
	pump := func(r io.ReadCloser, mkFrame func([]byte) *v2pb.ProcessFrame) {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				if wErr := write(mkFrame(append([]byte(nil), buf[:n]...))); wErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}
	wg.Add(2)
	go pump(stdoutP, func(b []byte) *v2pb.ProcessFrame {
		return &v2pb.ProcessFrame{Payload: &v2pb.ProcessFrame_Stdout{Stdout: b}}
	})
	go pump(stderrP, func(b []byte) *v2pb.ProcessFrame {
		return &v2pb.ProcessFrame{Payload: &v2pb.ProcessFrame_Stderr{Stderr: b}}
	})

	waitErr := cmd.Wait()
	wg.Wait() // drain pipes
	_ = ctx   // ctx cancellation already propagates via exec.CommandContext

	_ = write(&v2pb.ProcessFrame{Payload: &v2pb.ProcessFrame_Exit{Exit: waitInfo(cmd, waitErr)}})
	return nil
}

func waitInfo(cmd *exec.Cmd, waitErr error) *v2pb.ExitInfo {
	info := &v2pb.ExitInfo{}
	if waitErr == nil {
		return info
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		info.Code = int32(exitErr.ExitCode())
		if status := exitErr.ProcessState; status != nil && !status.Success() && !status.Exited() {
			// Signal kill: ExitCode is -1, surface the sys-level
			// signal name if Go makes it available.
			info.Signal = "killed"
		}
		return info
	}
	info.Code = -1
	info.Signal = waitErr.Error()
	return info
}

func writeOpenError(stream io.Writer, msg string) error {
	if err := link.WriteFrame(stream, &v2pb.ProcessOpenResponse{Error: "agent: " + msg}); err != nil {
		log.Debug("agent: process open error write: %v", err)
	}
	return nil
}

func killCmd(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
