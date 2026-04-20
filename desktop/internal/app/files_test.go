package app

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"

	keyring "github.com/zalando/go-keyring"
)

// fakeFiles backs an in-memory file map keyed by (sessionID, path) so
// tests can verify chunked round-trips without real PTY clients.
type fakeFiles struct {
	mu   map[string][]byte
	hits int32
}

func newFakeFiles() *fakeFiles { return &fakeFiles{mu: map[string][]byte{}} }

func (f *fakeFiles) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token":"tok"}`))
	})
	mux.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	mux.HandleFunc("/api/v1/sessions/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&f.hits, 1)
		// path is /api/v1/sessions/<id>/files[/size]
		path := r.URL.Path
		query := r.URL.Query()
		key := path + "|" + query.Get("path")

		switch {
		case r.Method == "GET" && (lastSeg(path) == "size"):
			data := f.mu[stripSize(key)]
			b, _ := json.Marshal(map[string]any{"status": true, "size": len(data)})
			w.Write(b)
		case r.Method == "GET":
			data := f.mu[key]
			off, _ := strconv.Atoi(query.Get("offset"))
			size, _ := strconv.Atoi(query.Get("size"))
			if off < 0 || off > len(data) {
				off = len(data)
			}
			end := len(data)
			if size > 0 && off+size < end {
				end = off + size
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(data[off:end])
		case r.Method == "POST":
			body, _ := io.ReadAll(r.Body)
			append := query.Get("append") == "true"
			if append {
				f.mu[key] = bytesAppend(f.mu[key], body)
			} else {
				f.mu[key] = body
			}
			b, _ := json.Marshal(map[string]any{"status": true, "bytes_written": len(body)})
			w.Write(b)
		default:
			t.Errorf("unexpected: %s %s", r.Method, path)
		}
	})
	return mux
}

func lastSeg(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}

func stripSize(key string) string {
	// "/api/v1/sessions/X/files/size|/etc/foo" → "/api/v1/sessions/X/files|/etc/foo"
	for i := 0; i < len(key); i++ {
		if i+5 <= len(key) && key[i:i+5] == "/size" {
			return key[:i] + key[i+5:]
		}
	}
	return key
}

func bytesAppend(a, b []byte) []byte { return append(append([]byte{}, a...), b...) }

func freshConnectedAppWithFakeFiles(t *testing.T, ff *fakeFiles) *App {
	t.Helper()
	srv := httptest.NewServer(ff.handler(t))
	t.Cleanup(srv.Close)

	keyring.MockInit()
	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-files-"+t.Name())
	a.emitFn = func(string, any) {}
	a.AddProfile("p", srv.URL, "s")
	if err := a.Connect("p"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(a.Disconnect)
	return a
}

func TestApp_FileSize_RoundTrip(t *testing.T) {
	ff := newFakeFiles()
	ff.mu["/api/v1/sessions/sid/files|/etc/hosts"] = []byte("127.0.0.1 localhost\n")
	a := freshConnectedAppWithFakeFiles(t, ff)
	got, err := a.FileSize("sid", "/etc/hosts")
	if err != nil {
		t.Fatal(err)
	}
	if got != int64(len("127.0.0.1 localhost\n")) {
		t.Errorf("size = %d", got)
	}
}

func TestApp_ReadFile_FullAndRange(t *testing.T) {
	ff := newFakeFiles()
	ff.mu["/api/v1/sessions/sid/files|/x"] = []byte("0123456789")
	a := freshConnectedAppWithFakeFiles(t, ff)

	got, err := a.ReadFile("sid", "/x", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "0123456789" {
		t.Errorf("full = %q", got)
	}

	got, err = a.ReadFile("sid", "/x", 3, 4)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "3456" {
		t.Errorf("range = %q", got)
	}
}

func TestApp_WriteFile_OverwriteAndAppend(t *testing.T) {
	ff := newFakeFiles()
	a := freshConnectedAppWithFakeFiles(t, ff)

	if err := a.WriteFile("sid", "/y", []byte("hello"), false); err != nil {
		t.Fatal(err)
	}
	if string(ff.mu["/api/v1/sessions/sid/files|/y"]) != "hello" {
		t.Errorf("after write: %q", ff.mu["/api/v1/sessions/sid/files|/y"])
	}

	if err := a.WriteFile("sid", "/y", []byte(" world"), true); err != nil {
		t.Fatal(err)
	}
	if string(ff.mu["/api/v1/sessions/sid/files|/y"]) != "hello world" {
		t.Errorf("after append: %q", ff.mu["/api/v1/sessions/sid/files|/y"])
	}
}

func TestApp_DownloadFile_ChunksLargeFile(t *testing.T) {
	ff := newFakeFiles()
	// 1.5 MiB random payload — exercises multiple 256 KiB chunks.
	payload := make([]byte, int(1.5*1024*1024))
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	}
	ff.mu["/api/v1/sessions/sid/files|/big.bin"] = payload

	a := freshConnectedAppWithFakeFiles(t, ff)
	dst := filepath.Join(t.TempDir(), "out.bin")
	if err := a.DownloadFile("sid", "/big.bin", dst); err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("downloaded len=%d != original len=%d", len(got), len(payload))
	}
	// Should have hit the server multiple times: size + several chunks.
	if atomic.LoadInt32(&ff.hits) < 3 {
		t.Errorf("hits = %d, expected chunked download", ff.hits)
	}
}

func TestApp_UploadFile_ChunksLargeFile(t *testing.T) {
	ff := newFakeFiles()
	a := freshConnectedAppWithFakeFiles(t, ff)

	src := filepath.Join(t.TempDir(), "in.bin")
	payload := make([]byte, 1228800) // 1.2 MiB
	rand.Read(payload)
	if err := os.WriteFile(src, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := a.UploadFile("sid", "/dst.bin", src); err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	got := ff.mu["/api/v1/sessions/sid/files|/dst.bin"]
	if !bytes.Equal(got, payload) {
		t.Errorf("uploaded len=%d != src len=%d", len(got), len(payload))
	}
}

func TestApp_FileOps_NotConnected(t *testing.T) {
	a := newTestApp(t)
	if _, err := a.FileSize("s", "/x"); err != ErrNotConnected {
		t.Errorf("FileSize err = %v", err)
	}
	if _, err := a.ReadFile("s", "/x", 0, 0); err != ErrNotConnected {
		t.Errorf("ReadFile err = %v", err)
	}
	if err := a.WriteFile("s", "/x", nil, false); err != ErrNotConnected {
		t.Errorf("WriteFile err = %v", err)
	}
}
