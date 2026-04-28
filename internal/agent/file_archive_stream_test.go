package agent

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// HandleFileArchiveStream packs the named paths on the agent's
// filesystem into the requested archive format and streams the
// bytes back as FileChunk frames. First frame is a
// FileArchiveResponse ack — empty error means the agent has opened
// every requested root and is about to start streaming bytes; a
// non-empty error closes the stream immediately.

// readArchiveBody reads frames until eof and returns the
// concatenated payload. Helper shared by the format-specific tests.
func readArchiveBody(t *testing.T, client io.Reader) []byte {
	t.Helper()
	var hdr v2pb.FileArchiveResponse
	if err := link.ReadFrame(client, &hdr); err != nil {
		t.Fatalf("read header: %v", err)
	}
	if hdr.Error != "" {
		t.Fatalf("header error: %s", hdr.Error)
	}
	var got []byte
	for {
		var ch v2pb.FileChunk
		if err := link.ReadFrame(client, &ch); err != nil {
			t.Fatalf("read chunk: %v", err)
		}
		got = append(got, ch.Data...)
		if ch.Eof {
			if ch.Error != "" {
				t.Fatalf("final chunk error: %s", ch.Error)
			}
			return got
		}
	}
}

// listTarEntries returns a sorted list of "name:size" pairs.
func listTarEntries(t *testing.T, body []byte) []string {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(body))
	var entries []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar.Next: %v", err)
		}
		buf, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read entry %s: %v", hdr.Name, err)
		}
		// For directories, tar header Size is 0 even though we
		// still emit a header.
		entries = append(entries, hdr.Name+":"+itoa(int64(len(buf))))
	}
	sort.Strings(entries)
	return entries
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// Happy path: a directory with two files round-trips through the
// uncompressed tar format.
func TestHandleFileArchiveStream_Tar(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha"), 0o644); err != nil {
		t.Fatalf("seed a.txt: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("seed sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("beta"), 0o644); err != nil {
		t.Fatalf("seed b.txt: %v", err)
	}

	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HandleFileArchiveStream(ctx, server, &v2pb.FileArchiveRequest{
			Paths:  []string{dir},
			Format: v2pb.ArchiveFormat_ARCHIVE_FORMAT_TAR,
		})
	}()

	body := readArchiveBody(t, client)
	entries := listTarEntries(t, body)

	// We expect base/a.txt (5), base/sub/ (0), base/sub/b.txt (4).
	// Names are prefixed by the basename of the root dir.
	base := filepath.Base(dir)
	want := []string{
		base + "/:0",
		base + "/a.txt:5",
		base + "/sub/:0",
		base + "/sub/b.txt:4",
	}
	if !equalStrings(entries, want) {
		t.Fatalf("entries = %v\nwant = %v", entries, want)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("handler returned: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("handler did not return")
	}
}

// Happy path: tar.gz round-trips and decompresses to the same
// content.
func TestHandleFileArchiveStream_TarGz(t *testing.T) {
	dir := t.TempDir()
	want := bytes.Repeat([]byte("platypus\n"), 4096) // ~36 KiB
	if err := os.WriteFile(filepath.Join(dir, "f"), want, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HandleFileArchiveStream(ctx, server, &v2pb.FileArchiveRequest{
			Paths:  []string{dir},
			Format: v2pb.ArchiveFormat_ARCHIVE_FORMAT_TAR_GZ,
		})
	}()

	body := readArchiveBody(t, client)

	// Decompress.
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var found []byte
	base := filepath.Base(dir)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar.Next: %v", err)
		}
		if hdr.Name == base+"/f" {
			found, err = io.ReadAll(tr)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
		}
	}
	if !bytes.Equal(found, want) {
		t.Fatalf("payload mismatch (got %d bytes, want %d)", len(found), len(want))
	}
	// Compression should buy us something on a repeating payload.
	if len(body) >= len(want) {
		t.Errorf("expected gzip to shrink payload: gz=%d raw=%d", len(body), len(want))
	}

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("handler did not return")
	}
}

// Happy path: zip format produces a parseable archive.
func TestHandleFileArchiveStream_Zip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("world!"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HandleFileArchiveStream(ctx, server, &v2pb.FileArchiveRequest{
			Paths:  []string{dir},
			Format: v2pb.ArchiveFormat_ARCHIVE_FORMAT_ZIP,
		})
	}()

	body := readArchiveBody(t, client)

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	got := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		buf, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		got[f.Name] = string(buf)
	}
	base := filepath.Base(dir)
	if got[base+"/a.txt"] != "hello" {
		t.Errorf("a.txt = %q; want %q", got[base+"/a.txt"], "hello")
	}
	if got[base+"/b.txt"] != "world!" {
		t.Errorf("b.txt = %q; want %q", got[base+"/b.txt"], "world!")
	}

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("handler did not return")
	}
}

// Empty paths list yields a populated header error and no chunks.
func TestHandleFileArchiveStream_EmptyPaths(t *testing.T) {
	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HandleFileArchiveStream(ctx, server, &v2pb.FileArchiveRequest{
			Format: v2pb.ArchiveFormat_ARCHIVE_FORMAT_TAR,
		})
	}()

	var hdr v2pb.FileArchiveResponse
	if err := link.ReadFrame(client, &hdr); err != nil {
		t.Fatalf("read header: %v", err)
	}
	if hdr.Error == "" {
		t.Fatal("expected error for empty paths")
	}

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("handler did not return")
	}
}

// A missing root yields a populated header error.
func TestHandleFileArchiveStream_MissingRoot(t *testing.T) {
	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HandleFileArchiveStream(ctx, server, &v2pb.FileArchiveRequest{
			Paths:  []string{"/platypus/does/not/exist"},
			Format: v2pb.ArchiveFormat_ARCHIVE_FORMAT_TAR,
		})
	}()

	var hdr v2pb.FileArchiveResponse
	if err := link.ReadFrame(client, &hdr); err != nil {
		t.Fatalf("read header: %v", err)
	}
	if hdr.Error == "" {
		t.Fatal("expected error for missing root")
	}

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("handler did not return")
	}
}

// Single-file root packs as one tar entry.
func TestHandleFileArchiveStream_SingleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "only.bin")
	if err := os.WriteFile(path, []byte("solo"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HandleFileArchiveStream(ctx, server, &v2pb.FileArchiveRequest{
			Paths:  []string{path},
			Format: v2pb.ArchiveFormat_ARCHIVE_FORMAT_TAR,
		})
	}()

	body := readArchiveBody(t, client)
	entries := listTarEntries(t, body)
	want := []string{"only.bin:4"}
	if !equalStrings(entries, want) {
		t.Fatalf("entries = %v; want %v", entries, want)
	}

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("handler did not return")
	}
}

// Cancellation mid-stream returns promptly.
func TestHandleFileArchiveStream_Cancel(t *testing.T) {
	// Write a "large" file so the handler is busy when we cancel.
	dir := t.TempDir()
	big := bytes.Repeat([]byte("x"), 4*1024*1024) // 4 MiB
	if err := os.WriteFile(filepath.Join(dir, "big.bin"), big, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- HandleFileArchiveStream(ctx, server, &v2pb.FileArchiveRequest{
			Paths:  []string{dir},
			Format: v2pb.ArchiveFormat_ARCHIVE_FORMAT_TAR,
		})
	}()

	// Read header so we know the agent has started writing.
	var hdr v2pb.FileArchiveResponse
	if err := link.ReadFrame(client, &hdr); err != nil {
		t.Fatalf("read header: %v", err)
	}
	cancel()
	// Drain a few frames so the writer side notices ctx died.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-done:
			return
		case <-deadline:
			t.Fatal("handler did not return after cancel")
		default:
		}
		var ch v2pb.FileChunk
		if err := link.ReadFrame(client, &ch); err != nil {
			// stream closed → handler should be returning soon.
			break
		}
		if ch.Eof {
			break
		}
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after cancel + drain")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
