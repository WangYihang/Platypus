package bridge_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/agent/plugin/bridge"
	"github.com/WangYihang/Platypus/internal/agent/plugin/system"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

func TestBridge_Stat_FileEntry(t *testing.T) {
	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	dir := t.TempDir()
	target := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(target, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	resp := bridge.Stat(reg)(context.Background(), &v2pb.StatRequest{Path: target})
	if resp.GetError() != "" {
		t.Fatalf("stat err: %s", resp.GetError())
	}
	e := resp.GetEntry()
	if e == nil {
		t.Fatalf("expected non-nil entry")
	}
	if e.GetName() != "hello.txt" {
		t.Errorf("name = %q", e.GetName())
	}
	if e.GetSize() != int64(len("hello world")) {
		t.Errorf("size = %d", e.GetSize())
	}
	if e.GetMode()&0o040000 != 0 {
		t.Errorf("regular file should not have dir bit set; mode = %o", e.GetMode())
	}
}

func TestBridge_Stat_DirectoryEntry(t *testing.T) {
	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	dir := t.TempDir()
	resp := bridge.Stat(reg)(context.Background(), &v2pb.StatRequest{Path: dir})
	if resp.GetError() != "" {
		t.Fatalf("stat err: %s", resp.GetError())
	}
	if resp.GetEntry().GetMode()&0o040000 == 0 {
		t.Errorf("directory should have dir bit set; mode = %o", resp.GetEntry().GetMode())
	}
}

func TestBridge_Stat_MissingPathErrors(t *testing.T) {
	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	resp := bridge.Stat(reg)(context.Background(),
		&v2pb.StatRequest{Path: "/this/path/definitely/does/not/exist"})
	if resp.GetError() == "" {
		t.Errorf("expected error for missing path, got entry=%+v", resp.GetEntry())
	}
}

// newRegWithSysPlugins is the shared fixture for bridge tests: fresh
// Registry rooted at a temp dir, with the bundled system plugins
// auto-installed. Mirrors what the agent does at boot.
func newRegWithSysPlugins(t *testing.T) *plugin.Registry {
	t.Helper()
	root := t.TempDir()
	reg, err := plugin.New(plugin.Options{Paths: plugin.NewPaths(root)})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	embFS, err := system.EmbeddedFS()
	if err != nil {
		t.Fatalf("EmbeddedFS: %v", err)
	}
	if r := system.EnsureInstalled(context.Background(), reg, embFS); len(r.Failed) > 0 {
		t.Fatalf("system bootstrap failures: %+v", r.Failed)
	}
	return reg
}
