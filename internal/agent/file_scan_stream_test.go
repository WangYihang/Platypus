package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// HandleFileScanStream walks the requested paths and emits a single
// FileScanResponse with file_count / dir_count / total_bytes. No
// file payload flows. The walk silently skips per-entry errors so
// a few unreadable subtrees still yield a useful estimate.

// Scanning a directory tree returns the right counts.
func TestHandleFileScanStream_DirectoryTotals(t *testing.T) {
	dir := t.TempDir()
	// Layout:
	//   dir/a.bin           (1024 bytes)
	//   dir/sub/             (dir)
	//   dir/sub/b.bin       (2048 bytes)
	//   dir/sub/empty/       (dir)
	if err := os.WriteFile(filepath.Join(dir, "a.bin"), make([]byte, 1024), 0o644); err != nil {
		t.Fatalf("seed a.bin: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub", "empty"), 0o755); err != nil {
		t.Fatalf("seed dirs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.bin"), make([]byte, 2048), 0o644); err != nil {
		t.Fatalf("seed b.bin: %v", err)
	}

	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HandleFileScanStream(ctx, server, &v2pb.FileScanRequest{
			Paths: []string{dir},
		})
	}()

	var resp v2pb.FileScanResponse
	if err := link.ReadFrame(client, &resp); err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.FileCount != 2 {
		t.Errorf("FileCount = %d; want 2", resp.FileCount)
	}
	// dir + sub + sub/empty = 3 directories.
	if resp.DirCount != 3 {
		t.Errorf("DirCount = %d; want 3", resp.DirCount)
	}
	if resp.TotalBytes != 1024+2048 {
		t.Errorf("TotalBytes = %d; want %d", resp.TotalBytes, 1024+2048)
	}

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("handler did not return")
	}
}

// Scanning a single regular file returns FileCount=1 and the right
// byte total.
func TestHandleFileScanStream_SingleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	if err := os.WriteFile(path, []byte("platypus"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HandleFileScanStream(ctx, server, &v2pb.FileScanRequest{
			Paths: []string{path},
		})
	}()

	var resp v2pb.FileScanResponse
	if err := link.ReadFrame(client, &resp); err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.FileCount != 1 || resp.DirCount != 0 || resp.TotalBytes != 8 {
		t.Errorf("got file=%d dir=%d bytes=%d; want 1 0 8",
			resp.FileCount, resp.DirCount, resp.TotalBytes)
	}

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("handler did not return")
	}
}

// Empty paths list returns an error and no counts.
func TestHandleFileScanStream_EmptyPaths(t *testing.T) {
	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HandleFileScanStream(ctx, server, &v2pb.FileScanRequest{})
	}()

	var resp v2pb.FileScanResponse
	if err := link.ReadFrame(client, &resp); err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.Error == "" {
		t.Fatal("expected error for empty paths")
	}

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("handler did not return")
	}
}

// A non-existent root yields a populated Error and no counts.
func TestHandleFileScanStream_MissingRoot(t *testing.T) {
	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HandleFileScanStream(ctx, server, &v2pb.FileScanRequest{
			Paths: []string{"/platypus/does/not/exist"},
		})
	}()

	var resp v2pb.FileScanResponse
	if err := link.ReadFrame(client, &resp); err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.Error == "" {
		t.Fatal("expected error for missing root")
	}

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("handler did not return")
	}
}

// Multiple roots accumulate.
func TestHandleFileScanStream_MultipleRoots(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	if err := os.WriteFile(a, []byte("aaa"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := os.WriteFile(b, []byte("bbbbbb"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HandleFileScanStream(ctx, server, &v2pb.FileScanRequest{
			Paths: []string{a, b},
		})
	}()

	var resp v2pb.FileScanResponse
	if err := link.ReadFrame(client, &resp); err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.FileCount != 2 || resp.TotalBytes != 9 {
		t.Errorf("got file=%d bytes=%d; want 2 9", resp.FileCount, resp.TotalBytes)
	}

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("handler did not return")
	}
}
