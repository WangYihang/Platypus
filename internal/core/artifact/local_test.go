package artifact_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WangYihang/Platypus/internal/core/artifact"
)

// happy: a manifest written to <root>/manifest/stable.json comes
// back through GetObject byte-for-byte.
func TestLocalStore_GetObjectHappy(t *testing.T) {
	root := t.TempDir()
	manifestDir := filepath.Join(root, "manifest")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	want := []byte(`{"version":"1.6.0","channel":"stable"}`)
	if err := os.WriteFile(filepath.Join(manifestDir, "stable.json"), want, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	s, err := artifact.NewLocalStore(root)
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	got, err := s.GetObject(context.Background(), "manifest/stable.json")
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("GetObject body = %q; want %q", got, want)
	}
}

// streaming reader returns size + content correctly.
func TestLocalStore_GetObjectReader(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "artifacts/1.6.0/linux/amd64")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	want := []byte("ELF\x7f...fake-binary-bytes")
	binPath := filepath.Join(binDir, "platypus-agent")
	if err := os.WriteFile(binPath, want, 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}

	s, err := artifact.NewLocalStore(root)
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	r, size, _, err := s.GetObjectReader(context.Background(), "artifacts/1.6.0/linux/amd64/platypus-agent")
	if err != nil {
		t.Fatalf("GetObjectReader: %v", err)
	}
	defer r.Close()
	if size != int64(len(want)) {
		t.Errorf("size = %d; want %d", size, len(want))
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("read mismatch")
	}
}

// missing object surfaces an os-style not-exist error so Distributor
// can map it to 404 (rather than 500 on every typo).
func TestLocalStore_MissingReturnsNotFound(t *testing.T) {
	root := t.TempDir()
	s, _ := artifact.NewLocalStore(root)
	_, err := s.GetObject(context.Background(), "manifest/stable.json")
	if err == nil {
		t.Fatal("expected error for missing object")
	}
	if !os.IsNotExist(err) && !strings.Contains(err.Error(), "no such file") {
		t.Errorf("error %v; want IsNotExist-ish", err)
	}
}

// path traversal: a key containing `..` must NOT escape the root.
// This is the only attack surface the local store has — without
// this check a poisoned manifest could read arbitrary files on
// disk.
func TestLocalStore_RejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	// Drop a file outside root that we don't want served.
	outside := filepath.Join(filepath.Dir(root), "leaked.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	defer os.Remove(outside)

	s, _ := artifact.NewLocalStore(root)
	for _, key := range []string{
		"../leaked.txt",
		"manifest/../../leaked.txt",
		"./../leaked.txt",
	} {
		_, err := s.GetObject(context.Background(), key)
		if err == nil {
			t.Errorf("key %q must be rejected as traversal", key)
		}
	}
}

// constructor rejects non-existent / non-directory roots so the
// operator notices at boot rather than at first agent self-upgrade.
func TestLocalStore_RejectsBadRoot(t *testing.T) {
	if _, err := artifact.NewLocalStore(""); err == nil {
		t.Error("empty root should be rejected")
	}
	if _, err := artifact.NewLocalStore("/nonexistent/platypus/releases"); err == nil {
		t.Error("missing root should be rejected")
	}
	// File posing as a root.
	root := t.TempDir()
	notDir := filepath.Join(root, "regular-file")
	_ = os.WriteFile(notDir, []byte{}, 0o600)
	if _, err := artifact.NewLocalStore(notDir); err == nil {
		t.Error("regular-file root should be rejected")
	}
}

// Prefix is empty for local stores (no bucket-sharing layer).
func TestLocalStore_Prefix(t *testing.T) {
	root := t.TempDir()
	s, _ := artifact.NewLocalStore(root)
	if got := s.Prefix(); got != "" {
		t.Errorf("Prefix = %q; want empty", got)
	}
}
