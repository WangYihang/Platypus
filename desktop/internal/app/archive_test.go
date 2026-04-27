package app

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"testing"
)

// memFS is the in-memory fsClient stand-in for archive_test. Backed
// by a flat map of absolute paths → file metadata; directories are
// implied by any path that is a strict prefix of another. The
// readCalls slice records every size argument passed to ReadFile so
// the chunking test can verify a giant file is never pulled into
// memory in one shot.
type memFS struct {
	files     map[string]*memFile
	readCalls []int64
}

type memFile struct {
	data  []byte
	isDir bool
	mode  uint32
}

func newMemFS() *memFS {
	return &memFS{files: map[string]*memFile{
		"/": {isDir: true, mode: 0o755},
	}}
}

func (m *memFS) addDir(p string) {
	p = strings.TrimRight(p, "/")
	if p == "" {
		return
	}
	if _, ok := m.files[p]; !ok {
		m.files[p] = &memFile{isDir: true, mode: 0o755}
		m.addDir(path.Dir(p))
	}
}

func (m *memFS) addFile(p string, data []byte) {
	m.addDir(path.Dir(p))
	m.files[p] = &memFile{data: data, mode: 0o644}
}

func (m *memFS) StatFile(_, p string) (FileEntryDTO, error) {
	f, ok := m.files[p]
	if !ok {
		return FileEntryDTO{}, fmt.Errorf("stat %s: not found", p)
	}
	mode := f.mode
	if f.isDir {
		mode |= 1 << 31 // ModeDir; matches what list/stat downstream cares about
	}
	return FileEntryDTO{
		Name:        path.Base(p),
		Size:        int64(len(f.data)),
		Mode:        mode,
		IsDir:       f.isDir,
		ModTimeUnix: 0,
	}, nil
}

func (m *memFS) ListDir(_, dir string, _, _ int64) (ListDirResult, error) {
	d, ok := m.files[dir]
	if !ok || !d.isDir {
		return ListDirResult{}, fmt.Errorf("list %s: not a directory", dir)
	}
	var out []FileEntryDTO
	prefix := strings.TrimRight(dir, "/") + "/"
	for p, f := range m.files {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		// Direct child only — no nested entries from this dir's listing.
		rest := strings.TrimPrefix(p, prefix)
		if rest == "" || strings.Contains(rest, "/") {
			continue
		}
		mode := f.mode
		if f.isDir {
			mode |= 1 << 31
		}
		out = append(out, FileEntryDTO{
			Name:  rest,
			Size:  int64(len(f.data)),
			Mode:  mode,
			IsDir: f.isDir,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return ListDirResult{Entries: out, Total: int64(len(out)), EOF: true}, nil
}

func (m *memFS) ReadFile(_, p string, off, size int64) ([]byte, error) {
	m.readCalls = append(m.readCalls, size)
	f, ok := m.files[p]
	if !ok || f.isDir {
		return nil, errors.New("read: not a regular file")
	}
	if off >= int64(len(f.data)) {
		return nil, nil
	}
	end := int64(len(f.data))
	if size > 0 && off+size < end {
		end = off + size
	}
	return f.data[off:end], nil
}

// readArchiveTar walks any tar reader and returns name → contents.
func readArchiveTar(t *testing.T, tr *tar.Reader) map[string]string {
	t.Helper()
	out := map[string]string{}
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("tar.Next: %v", err)
		}
		if h.Typeflag == tar.TypeReg {
			b, err := io.ReadAll(tr)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			out[h.Name] = string(b)
		} else {
			out[h.Name] = "<dir>"
		}
	}
	return out
}

func TestWriteArchive_TarGzPacksTree(t *testing.T) {
	fs := newMemFS()
	fs.addFile("/etc/nginx/nginx.conf", []byte("server { listen 80; }"))
	fs.addFile("/etc/nginx/conf.d/site.conf", []byte("# site"))
	fs.addDir("/etc/nginx/empty")

	var buf bytes.Buffer
	if err := writeArchive(fs, "sid", []string{"/etc/nginx"}, &buf, "tar.gz"); err != nil {
		t.Fatalf("writeArchive: %v", err)
	}

	gz, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer gz.Close()
	got := readArchiveTar(t, tar.NewReader(gz))

	if got["nginx/nginx.conf"] != "server { listen 80; }" {
		t.Errorf("nginx.conf = %q", got["nginx/nginx.conf"])
	}
	if got["nginx/conf.d/site.conf"] != "# site" {
		t.Errorf("site.conf = %q", got["nginx/conf.d/site.conf"])
	}
	if got["nginx/empty/"] != "<dir>" && got["nginx/empty"] != "<dir>" {
		t.Errorf("empty dir missing; entries: %v", keysOf(got))
	}
}

func TestWriteArchive_PlainTar(t *testing.T) {
	fs := newMemFS()
	fs.addFile("/data/a.txt", []byte("AAA"))
	fs.addFile("/data/b.txt", []byte("BBB"))

	var buf bytes.Buffer
	if err := writeArchive(fs, "sid", []string{"/data"}, &buf, "tar"); err != nil {
		t.Fatalf("writeArchive: %v", err)
	}
	got := readArchiveTar(t, tar.NewReader(&buf))
	if got["data/a.txt"] != "AAA" {
		t.Errorf("a.txt = %q", got["data/a.txt"])
	}
	if got["data/b.txt"] != "BBB" {
		t.Errorf("b.txt = %q", got["data/b.txt"])
	}
}

func TestWriteArchive_Zip(t *testing.T) {
	fs := newMemFS()
	fs.addFile("/data/a.txt", []byte("AAA"))
	fs.addFile("/data/sub/b.txt", []byte("BBB"))

	var buf bytes.Buffer
	if err := writeArchive(fs, "sid", []string{"/data"}, &buf, "zip"); err != nil {
		t.Fatalf("writeArchive: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	got := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		got[f.Name] = string(b)
	}
	if got["data/a.txt"] != "AAA" {
		t.Errorf("a.txt = %q", got["data/a.txt"])
	}
	if got["data/sub/b.txt"] != "BBB" {
		t.Errorf("sub/b.txt = %q", got["data/sub/b.txt"])
	}
}

func TestWriteArchive_MultipleRoots(t *testing.T) {
	fs := newMemFS()
	fs.addFile("/a/x.txt", []byte("X"))
	fs.addFile("/b/y.txt", []byte("Y"))

	var buf bytes.Buffer
	if err := writeArchive(fs, "sid", []string{"/a", "/b"}, &buf, "tar"); err != nil {
		t.Fatalf("writeArchive: %v", err)
	}
	got := readArchiveTar(t, tar.NewReader(&buf))
	if got["a/x.txt"] != "X" {
		t.Errorf("a/x.txt missing")
	}
	if got["b/y.txt"] != "Y" {
		t.Errorf("b/y.txt missing")
	}
}

func TestWriteArchive_RejectsUnknownFormat(t *testing.T) {
	fs := newMemFS()
	fs.addFile("/x", []byte("x"))
	var buf bytes.Buffer
	err := writeArchive(fs, "sid", []string{"/x"}, &buf, "rar")
	if err == nil {
		t.Fatal("expected error for unknown format, got nil")
	}
}

// TestWriteArchive_ChunksLargeFiles is the DoS guard: even a 5 MiB
// file must be pulled in 256 KiB chunks (or less). If anyone refactors
// writeArchive to call ReadFile(off=0, size=0) on a huge file we'll
// see one big call and the test fails.
func TestWriteArchive_ChunksLargeFiles(t *testing.T) {
	fs := newMemFS()
	const fileSize = 5 * 1024 * 1024
	big := make([]byte, fileSize)
	if _, err := rand.Read(big); err != nil {
		t.Fatal(err)
	}
	fs.addFile("/d/big.bin", big)

	var buf bytes.Buffer
	if err := writeArchive(fs, "sid", []string{"/d"}, &buf, "tar"); err != nil {
		t.Fatalf("writeArchive: %v", err)
	}

	// Every read call must be bounded; we never want a single
	// "give me the whole file" call. The agent only enforces this if
	// the client respects the chunk size.
	const maxChunk = int64(256 * 1024)
	for _, sz := range fs.readCalls {
		if sz <= 0 || sz > maxChunk {
			t.Errorf("ReadFile size=%d (must be 0 < size <= %d)", sz, maxChunk)
		}
	}
	// And the contents should round-trip — chunking shouldn't
	// reorder bytes.
	got := readArchiveTar(t, tar.NewReader(&buf))
	if got["d/big.bin"] != string(big) {
		t.Errorf("big.bin contents do not round-trip")
	}
}

// TestWriteArchive_StreamsToDestination ensures writeArchive doesn't
// buffer the whole archive in memory before flushing — bytes appear
// on dst progressively. We use a counter writer that records the
// max-in-flight write size; any single Write() bigger than ~1 MiB
// would mean the implementation accumulated.
func TestWriteArchive_StreamsToDestination(t *testing.T) {
	fs := newMemFS()
	for i := 0; i < 10; i++ {
		blob := bytes.Repeat([]byte{byte(i)}, 256*1024)
		fs.addFile(fmt.Sprintf("/d/f%02d.bin", i), blob)
	}
	var counter writeCounter
	if err := writeArchive(fs, "sid", []string{"/d"}, &counter, "tar"); err != nil {
		t.Fatalf("writeArchive: %v", err)
	}
	// Loose ceiling: any single archive Write() must stay under 1 MiB
	// (worst case would be padding + header + chunk = a couple hundred
	// kilobytes). A 256 KiB chunk plus a 512 byte tar header is the
	// realistic peak.
	if counter.max > 1*1024*1024 {
		t.Errorf("largest single write was %d bytes — implementation buffered", counter.max)
	}
	if counter.total < 2*1024*1024 {
		t.Errorf("expected the archive to span multiple writes; total=%d", counter.total)
	}
}

type writeCounter struct {
	total int64
	max   int64
}

func (w *writeCounter) Write(p []byte) (int, error) {
	n := int64(len(p))
	w.total += n
	if n > w.max {
		w.max = n
	}
	return int(n), nil
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Sanity check: the App method that wraps writeArchive writes the
// archive to disk, doesn't blow up on tar.gz, and produces a valid
// gzip header. End-to-end smoke; the heavy lifting is covered by the
// unit-level tests above.
func TestApp_DownloadArchive_WritesToDisk(t *testing.T) {
	fs := newMemFS()
	fs.addFile("/d/x.txt", []byte("hi"))

	tmp := t.TempDir()
	dst := tmp + "/out.tar.gz"
	if err := downloadArchive(fs, "sid", []string{"/d"}, dst, "tar.gz"); err != nil {
		t.Fatalf("downloadArchive: %v", err)
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 2 || b[0] != 0x1f || b[1] != 0x8b {
		t.Errorf("expected gzip magic at start, got % x", b[:min(2, len(b))])
	}
}
