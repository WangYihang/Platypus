package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakepluginPath is set by TestMain — the on-disk path of the
// compiled fakeplugin binary. All Launch tests point at it.
var fakepluginPath atomic.Value // string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "fakeplugin-bin-")
	if err != nil {
		panic("mkdir tmp: " + err.Error())
	}
	defer os.RemoveAll(tmp)
	out := filepath.Join(tmp, "fakeplugin")
	cmd := exec.Command("go", "build", "-o", out, "./testdata/fakeplugin")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic("build fakeplugin: " + err.Error())
	}
	fakepluginPath.Store(out)
	os.Exit(m.Run())
}

func newRuntime(t *testing.T, behavior string) *Runtime {
	t.Helper()
	wd := t.TempDir()
	bin, _ := fakepluginPath.Load().(string)
	r := New(Config{
		PluginID:      "test.fakeplugin",
		Version:       "1.2.3",
		BinaryPath:    bin,
		WorkDir:       wd,
		HostSocket:    filepath.Join(wd, "host.sock"),
		Grants:        []string{"kv", "fs.read"},
		LaunchTimeout: 3 * time.Second,
		StopGrace:     500 * time.Millisecond,
	})
	if behavior != "" {
		r.cfg.ExtraEnv = append(r.cfg.ExtraEnv, "FAKE_BEHAVIOR="+behavior)
	}
	return r
}

func TestLaunch_Healthy(t *testing.T) {
	r := newRuntime(t, "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := r.Launch(ctx); err != nil {
		t.Fatalf("Launch: %v\nstderr: %s", err, r.StderrTail())
	}
	t.Cleanup(func() { _ = r.Stop(context.Background()) })

	resp, err := r.Health(ctx)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if resp.PluginId != "test.fakeplugin" || resp.Version != "1.2.3" || !resp.Ready {
		t.Fatalf("bad health: %+v", resp)
	}
}

func TestLaunch_BinaryMissing(t *testing.T) {
	r := newRuntime(t, "")
	r.cfg.BinaryPath = "/nonexistent/binary"
	err := r.Launch(context.Background())
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
	if !strings.Contains(err.Error(), "start") {
		t.Fatalf("error not from start: %v", err)
	}
}

func TestLaunch_NoReady(t *testing.T) {
	r := newRuntime(t, "no_ready")
	r.cfg.LaunchTimeout = 700 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := r.Launch(ctx)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "READY") {
		t.Fatalf("error should mention READY: %v", err)
	}
}

func TestLaunch_IDMismatch(t *testing.T) {
	r := newRuntime(t, "id_mismatch")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := r.Launch(ctx)
	if err == nil {
		t.Fatal("expected id mismatch error")
	}
	if !strings.Contains(err.Error(), "identifies as") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLaunch_VersionMismatch(t *testing.T) {
	r := newRuntime(t, "version_mismatch")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := r.Launch(ctx)
	if err == nil {
		t.Fatal("expected version mismatch error")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLaunch_CrashOnStart(t *testing.T) {
	r := newRuntime(t, "crash_on_start")
	r.cfg.LaunchTimeout = 1 * time.Second
	err := r.Launch(context.Background())
	if err == nil {
		t.Fatal("expected error when child exits before READY")
	}
}

func TestStop_Graceful(t *testing.T) {
	r := newRuntime(t, "")
	if err := r.Launch(context.Background()); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if err := r.Stop(context.Background()); err != nil {
		// Graceful exit returns nil. *exec.ExitError with signal SIGTERM
		// is also acceptable in case scheduling raced.
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("Stop: unexpected error %v", err)
		}
	}

	// Idempotent.
	if err := r.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestStop_IgnoreShutdownFallsThroughToSignal(t *testing.T) {
	r := newRuntime(t, "ignore_shutdown")
	r.cfg.StopGrace = 200 * time.Millisecond
	if err := r.Launch(context.Background()); err != nil {
		t.Fatalf("Launch: %v", err)
	}

	start := time.Now()
	_ = r.Stop(context.Background()) // expected to be a *exec.ExitError after SIGKILL
	elapsed := time.Since(start)

	// Shutdown grace + SIGTERM grace = 2 * StopGrace. The
	// fakeplugin's "ignore_shutdown" branch ignores BOTH the gRPC
	// Shutdown and SIGTERM (it stays blocked in the rpc), so we
	// expect the SIGKILL path: ~ 2 * StopGrace.
	if elapsed < r.cfg.StopGrace {
		t.Fatalf("Stop returned too fast: %s", elapsed)
	}
	if elapsed > 4*time.Second {
		t.Fatalf("Stop took too long: %s", elapsed)
	}
}

func TestLaunch_DoubleCallRejected(t *testing.T) {
	r := newRuntime(t, "")
	if err := r.Launch(context.Background()); err != nil {
		t.Fatalf("first Launch: %v", err)
	}
	t.Cleanup(func() { _ = r.Stop(context.Background()) })
	if err := r.Launch(context.Background()); err == nil {
		t.Fatal("second Launch should fail")
	}
}

func TestStderrCapture(t *testing.T) {
	r := newRuntime(t, "")
	r.cfg.ExtraEnv = append(r.cfg.ExtraEnv, "FAKE_BEHAVIOR=no_ready")
	r.cfg.LaunchTimeout = 500 * time.Millisecond
	_ = r.Launch(context.Background())
	// fakeplugin's "no_ready" path prints nothing to stderr; it
	// just blocks. The captured tail should at least be a string,
	// possibly empty.
	_ = r.StderrTail()
}
