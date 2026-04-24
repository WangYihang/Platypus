package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// Production RPC handlers for the filesystem-shaped RPCs declared in
// proto/v2/rpc.proto. Each is a thin os/syscall wrapper that rides on
// the AgentRPCHandlers struct.

func TestHandleListDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a"), []byte("1"), 0o644)
	os.WriteFile(filepath.Join(dir, "b"), []byte("22"), 0o644)

	resp := HandleListDir(context.Background(), &v2pb.ListDirRequest{Path: dir})
	if resp.Error != "" {
		t.Fatalf("err: %s", resp.Error)
	}
	if len(resp.Entries) != 2 {
		t.Fatalf("entries=%d; want 2", len(resp.Entries))
	}
	names := map[string]int64{}
	for _, e := range resp.Entries {
		names[e.Name] = e.Size
	}
	if names["a"] != 1 || names["b"] != 2 {
		t.Fatalf("sizes mismatch: %+v", names)
	}
}

func TestHandleListDir_NotFound(t *testing.T) {
	resp := HandleListDir(context.Background(), &v2pb.ListDirRequest{Path: "/nope"})
	if resp.Error == "" {
		t.Fatal("want error on missing path")
	}
}

func TestHandleStat(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "hello")
	os.WriteFile(f, []byte("hi"), 0o600)
	resp := HandleStat(context.Background(), &v2pb.StatRequest{Path: f})
	if resp.Error != "" {
		t.Fatalf("err: %s", resp.Error)
	}
	if resp.Entry == nil || resp.Entry.Size != 2 {
		t.Fatalf("entry=%+v; want size=2", resp.Entry)
	}
}

func TestHandleDelete(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "gone")
	os.WriteFile(f, []byte("x"), 0o644)

	resp := HandleDelete(context.Background(), &v2pb.DeleteRequest{Path: f})
	if resp.Error != "" {
		t.Fatalf("err: %s", resp.Error)
	}
	if _, err := os.Stat(f); err == nil {
		t.Fatal("file still exists after Delete")
	}
}

func TestHandleDelete_Recursive(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a/b/c"), 0o755)
	os.WriteFile(filepath.Join(dir, "a/b/c/leaf"), []byte("x"), 0o644)

	resp := HandleDelete(context.Background(), &v2pb.DeleteRequest{
		Path: filepath.Join(dir, "a"), Recursive: true,
	})
	if resp.Error != "" {
		t.Fatalf("err: %s", resp.Error)
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); err == nil {
		t.Fatal("tree still exists")
	}
}

func TestHandleRenameV2(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	os.WriteFile(src, []byte("x"), 0o644)
	resp := HandleRename(context.Background(), &v2pb.RenameRequest{From: src, To: dst})
	if resp.Error != "" {
		t.Fatalf("err: %s", resp.Error)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("dst missing: %v", err)
	}
}

func TestHandleMkdir(t *testing.T) {
	dir := t.TempDir()
	d := filepath.Join(dir, "new")
	resp := HandleMkdir(context.Background(), &v2pb.MkdirRequest{Path: d, Mode: 0o755})
	if resp.Error != "" {
		t.Fatalf("err: %s", resp.Error)
	}
	info, err := os.Stat(d)
	if err != nil || !info.IsDir() {
		t.Fatalf("stat: %v isDir=%v", err, info.IsDir())
	}
}

func TestHandleMkdir_RecursiveFlag(t *testing.T) {
	dir := t.TempDir()
	d := filepath.Join(dir, "a/b/c")
	resp := HandleMkdir(context.Background(), &v2pb.MkdirRequest{Path: d, Mkdirs: true})
	if resp.Error != "" {
		t.Fatalf("err: %s", resp.Error)
	}
	if _, err := os.Stat(d); err != nil {
		t.Fatalf("tree not created: %v", err)
	}
}

func TestHandleChmod(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "m")
	os.WriteFile(f, []byte("x"), 0o644)
	resp := HandleChmod(context.Background(), &v2pb.ChmodRequest{Path: f, Mode: 0o600})
	if resp.Error != "" {
		t.Fatalf("err: %s", resp.Error)
	}
	info, _ := os.Stat(f)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o; want 0600", info.Mode().Perm())
	}
}

func TestHandleSysInfo(t *testing.T) {
	resp := HandleSysInfo(context.Background(), &v2pb.SysInfoRequest{})
	if resp.Error != "" {
		t.Fatalf("err: %s", resp.Error)
	}
	if resp.Os == "" || resp.Arch == "" {
		t.Fatalf("os=%q arch=%q both should be populated", resp.Os, resp.Arch)
	}
	if !strings.Contains(resp.Hostname, "") {
		// hostname can be empty in some sandboxes, don't fail — just
		// assert the SysInfoResponse exists.
	}
}
