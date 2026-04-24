package agent

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// HandleProcessStream is the agent-side handler for a
// STREAM_TYPE_PROCESS_OPEN stream. It spawns the child (with a PTY
// when req.Pty), writes a ProcessOpenResponse as the first frame,
// pumps stdin ← server → PTY and PTY → stdout → server, emits a
// final ExitInfo frame, and closes.

// Paired streams give us a client-side stream that talks to the
// agent handler directly — no yamux needed.
func pairedProcessStreams(t *testing.T) (client, server io.ReadWriteCloser) {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	return clientConn, serverConn
}

// Happy path (non-PTY mode): run `echo hi`, expect stdout frame with
// "hi\n" then an ExitInfo frame with code 0.
func TestHandleProcessStream_NonPTYEcho(t *testing.T) {
	client, server := pairedProcessStreams(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req := &v2pb.ProcessOpenRequest{
		Command: "echo",
		Args:    []string{"hi"},
		Pty:     false,
	}

	done := make(chan error, 1)
	go func() {
		done <- HandleProcessStream(ctx, server, req)
	}()

	// First frame must be ProcessOpenResponse.
	var openResp v2pb.ProcessOpenResponse
	if err := link.ReadFrame(client, &openResp); err != nil {
		t.Fatalf("read open response: %v", err)
	}
	if openResp.Error != "" {
		t.Fatalf("open response error: %s", openResp.Error)
	}
	if openResp.Pid <= 0 {
		t.Fatalf("pid = %d; want > 0", openResp.Pid)
	}

	// Drain subsequent frames until we see an ExitInfo.
	var gotStdout []byte
	var exitCode int32 = -1
	for {
		var f v2pb.ProcessFrame
		if err := link.ReadFrame(client, &f); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
				break
			}
			t.Fatalf("read frame: %v", err)
		}
		switch p := f.Payload.(type) {
		case *v2pb.ProcessFrame_Stdout:
			gotStdout = append(gotStdout, p.Stdout...)
		case *v2pb.ProcessFrame_Exit:
			exitCode = p.Exit.Code
			goto drained
		}
	}
drained:

	if exitCode != 0 {
		t.Fatalf("exit code = %d; want 0", exitCode)
	}
	if string(gotStdout) != "hi\n" {
		t.Fatalf("stdout = %q; want %q", gotStdout, "hi\n")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("HandleProcessStream: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("HandleProcessStream did not return")
	}
}

// Spawn failure (missing binary): the agent writes
// ProcessOpenResponse.Error and closes without any ProcessFrame.
func TestHandleProcessStream_MissingBinaryReportsError(t *testing.T) {
	client, server := pairedProcessStreams(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req := &v2pb.ProcessOpenRequest{
		Command: "/platypus-test/never/exists",
		Pty:     false,
	}
	done := make(chan error, 1)
	go func() {
		done <- HandleProcessStream(ctx, server, req)
	}()

	var openResp v2pb.ProcessOpenResponse
	if err := link.ReadFrame(client, &openResp); err != nil {
		t.Fatalf("read open response: %v", err)
	}
	if openResp.Error == "" {
		t.Fatal("expected non-empty Error for missing binary")
	}

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("HandleProcessStream did not return after spawn failure")
	}
}

// PTY mode round-trip: spawn `cat`, send some bytes as stdin,
// expect the same bytes echoed back in stdout (cat's pty echo is
// the line-discipline default). Then close client → handler exits.
func TestHandleProcessStream_PTYStdinEchoesToStdout(t *testing.T) {
	if !ptyTestAvailable() {
		t.Skip("no /dev/ptmx available; PTY tests not supported on this runner")
	}
	client, server := pairedProcessStreams(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &v2pb.ProcessOpenRequest{
		Command: "cat",
		Pty:     true,
		Cols:    80,
		Rows:    24,
	}
	done := make(chan error, 1)
	go func() {
		done <- HandleProcessStream(ctx, server, req)
	}()

	// Open response.
	var openResp v2pb.ProcessOpenResponse
	if err := link.ReadFrame(client, &openResp); err != nil {
		t.Fatalf("read open response: %v", err)
	}
	if openResp.Error != "" {
		t.Fatalf("open error: %s", openResp.Error)
	}

	// Send a stdin frame with "ping\n".
	if err := link.WriteFrame(client, &v2pb.ProcessFrame{
		Payload: &v2pb.ProcessFrame_Stdin{Stdin: []byte("ping\n")},
	}); err != nil {
		t.Fatalf("write stdin: %v", err)
	}

	// Drain stdout until we see "ping" (cat echoes typed chars plus
	// repeats the line on newline under the default line discipline).
	deadline := time.Now().Add(2 * time.Second)
	var seen []byte
	for time.Now().Before(deadline) {
		var f v2pb.ProcessFrame
		if err := link.ReadFrame(client, &f); err != nil {
			t.Fatalf("read frame: %v", err)
		}
		if out := f.GetStdout(); out != nil {
			seen = append(seen, out...)
			if containsBytes(seen, []byte("ping")) {
				break
			}
			continue
		}
	}
	if !containsBytes(seen, []byte("ping")) {
		t.Fatalf("stdin did not echo into stdout; got %q", seen)
	}

	// Close the client side → handler exits.
	_ = client.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("HandleProcessStream did not return after client close")
	}
}

func ptyTestAvailable() bool {
	// Cheap detect: try opening /dev/ptmx.
	f, err := os.Open("/dev/ptmx")
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func containsBytes(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
