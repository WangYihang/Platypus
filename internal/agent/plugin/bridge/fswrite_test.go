package bridge_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin/bridge"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

func TestBridge_Mkdir_CreatesDirectory(t *testing.T) {
	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	parent := t.TempDir()
	target := filepath.Join(parent, "new-dir")
	resp := bridge.Mkdir(reg)(context.Background(),
		&v2pb.MkdirRequest{Path: target, Mode: 0o755})
	if resp.GetError() != "" {
		t.Fatalf("mkdir err: %s", resp.GetError())
	}
	st, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat after mkdir: %v", err)
	}
	if !st.IsDir() {
		t.Errorf("expected directory")
	}
}

func TestBridge_Mkdir_NestedRequiresMkdirs(t *testing.T) {
	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	// Without mkdirs: parent missing → fails
	resp := bridge.Mkdir(reg)(context.Background(),
		&v2pb.MkdirRequest{Path: deep, Mode: 0o755})
	if resp.GetError() == "" {
		t.Errorf("expected error for nested mkdir without mkdirs")
	}
	// With mkdirs: succeeds
	resp = bridge.Mkdir(reg)(context.Background(),
		&v2pb.MkdirRequest{Path: deep, Mode: 0o755, Mkdirs: true})
	if resp.GetError() != "" {
		t.Errorf("mkdir mkdirs=true: %s", resp.GetError())
	}
}

func TestBridge_Chmod_ChangesMode(t *testing.T) {
	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	dir := t.TempDir()
	target := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(target, []byte("data"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	resp := bridge.Chmod(reg)(context.Background(),
		&v2pb.ChmodRequest{Path: target, Mode: 0o644})
	if resp.GetError() != "" {
		t.Fatalf("chmod err: %s", resp.GetError())
	}
	st, _ := os.Stat(target)
	if st.Mode().Perm() != 0o644 {
		t.Errorf("mode = %o, want 0644", st.Mode().Perm())
	}
}

func TestBridge_Delete_RemovesFileAndTree(t *testing.T) {
	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	resp := bridge.Delete(reg)(context.Background(),
		&v2pb.DeleteRequest{Path: file})
	if resp.GetError() != "" {
		t.Fatalf("delete file: %s", resp.GetError())
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("file still present after delete: %v", err)
	}

	// Recursive delete of a populated tree.
	tree := filepath.Join(dir, "tree")
	_ = os.MkdirAll(filepath.Join(tree, "sub"), 0o755)
	_ = os.WriteFile(filepath.Join(tree, "sub", "x"), []byte("y"), 0o600)
	resp = bridge.Delete(reg)(context.Background(),
		&v2pb.DeleteRequest{Path: tree, Recursive: true})
	if resp.GetError() != "" {
		t.Fatalf("recursive delete: %s", resp.GetError())
	}
	if _, err := os.Stat(tree); !os.IsNotExist(err) {
		t.Errorf("tree still present: %v", err)
	}
}

func TestBridge_Rename_MovesFile(t *testing.T) {
	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	dir := t.TempDir()
	from := filepath.Join(dir, "old.txt")
	to := filepath.Join(dir, "new.txt")
	if err := os.WriteFile(from, []byte("body"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	resp := bridge.Rename(reg)(context.Background(),
		&v2pb.RenameRequest{From: from, To: to})
	if resp.GetError() != "" {
		t.Fatalf("rename: %s", resp.GetError())
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Errorf("from still present: %v", err)
	}
	got, err := os.ReadFile(to)
	if err != nil {
		t.Fatalf("read to: %v", err)
	}
	if string(got) != "body" {
		t.Errorf("contents = %q", got)
	}
}
