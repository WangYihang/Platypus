package agent

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// fakeDistributor stands in for the server's distributor routes
// during agent-side upgrade tests. The release pipeline normally
// uploads manifest + sig + binary to S3 and the server proxies
// reads back; for unit tests we just serve hand-built bytes
// straight from memory.
type fakeDistributor struct {
	manifest     []byte
	signature    []byte
	binary       []byte
	binarySHA256 string

	// Knobs for negative-path tests.
	manifestStatus int // 0 → 200
	sigStatus      int // 0 → 200
	binStatus      int // 0 → 200
}

func (f *fakeDistributor) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/manifest/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case len(r.URL.Path) >= len("/v1/manifest/") && r.URL.Path[len("/v1/manifest/"):] == "stable/signature":
			if f.sigStatus != 0 {
				http.Error(w, "boom", f.sigStatus)
				return
			}
			_, _ = w.Write(f.signature)
		case r.URL.Path == "/v1/manifest/stable":
			if f.manifestStatus != 0 {
				http.Error(w, "boom", f.manifestStatus)
				return
			}
			_, _ = w.Write(f.manifest)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/v1/artifacts/", func(w http.ResponseWriter, r *http.Request) {
		if f.binStatus != 0 {
			http.Error(w, "boom", f.binStatus)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(f.binary)))
		_, _ = w.Write(f.binary)
	})
	return mux
}

// buildFakeRelease creates an Ed25519 keypair, signs a manifest for
// the running test platform, and returns everything the runner needs
// to walk one happy-path upgrade. binaryBytes is the payload that
// will be written into BinaryPath after the upgrade — tests assert
// it landed by reading the file back.
func buildFakeRelease(t *testing.T, version string) (*fakeDistributor, string, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}

	binary := []byte("#!/bin/sh\necho fake-agent-" + version + "\n")
	sum := sha256.Sum256(binary)

	manifest := distributorManifest{
		Version:    version,
		Channel:    "stable",
		ReleasedAt: time.Now().UTC(),
		Artifacts: []distributorArtifact{{
			OS:     runtime.GOOS,
			Arch:   runtime.GOARCH,
			Key:    "platypus-agent-" + version,
			Size:   int64(len(binary)),
			SHA256: hex.EncodeToString(sum[:]),
		}},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	sig := ed25519.Sign(priv, manifestBytes)
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	return &fakeDistributor{
		manifest:     manifestBytes,
		signature:    []byte(sigB64),
		binary:       binary,
		binarySHA256: hex.EncodeToString(sum[:]),
	}, base64.StdEncoding.EncodeToString(pub), priv
}

// readAllProgress drains the client side of the paired stream until
// it sees a terminal phase. Returns the slice of frames in order so
// tests can assert phase progression and final state.
func readAllProgress(t *testing.T, r *bytes.Buffer, until time.Time) []*v2pb.UpgradeProgress {
	t.Helper()
	var out []*v2pb.UpgradeProgress
	for time.Now().Before(until) {
		var p v2pb.UpgradeProgress
		if err := link.ReadFrame(r, &p); err != nil {
			break
		}
		out = append(out, &p)
		if p.Phase == v2pb.UpgradeProgress_PHASE_EXITING ||
			p.Phase == v2pb.UpgradeProgress_PHASE_FAILED {
			break
		}
	}
	return out
}

// runUpgrade wires up the runner against a fake distributor, drives
// one Handle() call, and returns the parsed progress frames + the
// post-upgrade state of the binary path.
func runUpgrade(t *testing.T, fd *fakeDistributor, pubB64 string, req *v2pb.AgentUpgradeRequest) (
	progress []*v2pb.UpgradeProgress,
	binaryPath string,
	exitCode *int,
	err error,
) {
	t.Helper()

	srv := httptest.NewTLSServer(fd.handler())
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	binaryPath = filepath.Join(dir, "platypus-agent")
	if err := os.WriteFile(binaryPath, []byte("OLD"), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Capture exit code instead of dying.
	var exitMu sync.Mutex
	var captured *int

	// One end of an in-memory full-duplex byte stream feeds the
	// runner (it writes progress here); the other end is read in
	// the test thread.
	pipeRead := &bytes.Buffer{}
	pipeWrite := &threadSafeWriter{w: pipeRead}

	runner := &UpgradeRunner{
		DistributorBaseURL:  srv.URL,
		HTTPClient:          srv.Client(),
		SigningPublicKeyB64: pubB64,
		BinaryPath:          binaryPath,
		ExitFn: func(code int) {
			exitMu.Lock()
			defer exitMu.Unlock()
			c := code
			captured = &c
		},
		Now: time.Now,
	}

	stream := &readWriteCloser{Writer: pipeWrite, Reader: &bytes.Buffer{} /* runner only writes */}
	err = runner.Handle(context.Background(), stream, req)

	progress = readAllProgress(t, pipeRead, time.Now().Add(2*time.Second))
	exitCode = captured
	return
}

// threadSafeWriter serializes concurrent writes from runner ↔
// progressReader so the bytes.Buffer underneath stays consistent.
type threadSafeWriter struct {
	mu sync.Mutex
	w  *bytes.Buffer
}

func (t *threadSafeWriter) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.w.Write(b)
}

// readWriteCloser glues a Writer + Reader into the io.ReadWriteCloser
// that Handle expects. Close is a no-op because the in-memory buffer
// has no underlying resource.
type readWriteCloser struct {
	Writer interface {
		Write([]byte) (int, error)
	}
	Reader interface {
		Read([]byte) (int, error)
	}
}

func (r *readWriteCloser) Write(b []byte) (int, error) { return r.Writer.Write(b) }
func (r *readWriteCloser) Read(b []byte) (int, error)  { return r.Reader.Read(b) }
func (r *readWriteCloser) Close() error                { return nil }

func TestUpgradeRunner_HappyPath(t *testing.T) {
	fd, pub, _ := buildFakeRelease(t, "1.6.0")

	// HACK: the runner sleeps 100ms after PHASE_EXITING to let the
	// frame flush before exiting. Tests don't care about that flush
	// but they do block on ExitFn — we replace ExitFn with a no-op
	// capture so the goroutine returns promptly.
	progress, binaryPath, exitCode, err := runUpgrade(t, fd, pub, &v2pb.AgentUpgradeRequest{
		TargetVersion: "1.6.0",
		Channel:       "stable",
		Actor:         "user:tester",
	})

	if err != nil {
		t.Fatalf("Handle: unexpected error: %v", err)
	}
	if exitCode == nil || *exitCode != 75 {
		t.Fatalf("ExitFn not called with 75; got %v", exitCode)
	}

	// We expect to see at minimum: FETCH_MANIFEST → VERIFY_SIG →
	// DOWNLOAD → VERIFY_SHA256 → INSTALL → EXITING. The download
	// may emit extra DOWNLOAD frames depending on tick timing;
	// tolerate that.
	expectedTerminal := v2pb.UpgradeProgress_PHASE_EXITING
	gotTerminal := progress[len(progress)-1].Phase
	if gotTerminal != expectedTerminal {
		t.Fatalf("terminal phase = %v; want %v; frames = %+v",
			gotTerminal, expectedTerminal, progress)
	}
	wantPhases := []v2pb.UpgradeProgress_Phase{
		v2pb.UpgradeProgress_PHASE_FETCH_MANIFEST,
		v2pb.UpgradeProgress_PHASE_VERIFY_SIG,
		v2pb.UpgradeProgress_PHASE_DOWNLOAD,
		v2pb.UpgradeProgress_PHASE_VERIFY_SHA256,
		v2pb.UpgradeProgress_PHASE_INSTALL,
		v2pb.UpgradeProgress_PHASE_EXITING,
	}
	seen := map[v2pb.UpgradeProgress_Phase]bool{}
	for _, p := range progress {
		seen[p.Phase] = true
	}
	for _, want := range wantPhases {
		if !seen[want] {
			t.Errorf("missing phase %v in progress trace; got phases %v", want, progress)
		}
	}

	// Binary on disk should now be the new content.
	got, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("read installed binary: %v", err)
	}
	if !bytes.Contains(got, []byte("fake-agent-1.6.0")) {
		t.Fatalf("installed binary content = %q; want fake-agent-1.6.0", got)
	}

	// .bak should hold the old content.
	bak, err := os.ReadFile(binaryPath + ".bak")
	if err != nil {
		t.Fatalf("read .bak: %v", err)
	}
	if string(bak) != "OLD" {
		t.Fatalf(".bak content = %q; want OLD", bak)
	}
}

func TestUpgradeRunner_VersionMismatchRefuses(t *testing.T) {
	fd, pub, _ := buildFakeRelease(t, "1.6.0")

	progress, _, exitCode, err := runUpgrade(t, fd, pub, &v2pb.AgentUpgradeRequest{
		TargetVersion: "1.7.0", // manifest says 1.6.0, operator wanted 1.7.0
		Channel:       "stable",
	})
	if err == nil {
		t.Fatalf("Handle: expected error on version mismatch")
	}
	if exitCode != nil {
		t.Fatalf("ExitFn should NOT have been called on mismatch; got %v", *exitCode)
	}
	last := progress[len(progress)-1]
	if last.Phase != v2pb.UpgradeProgress_PHASE_FAILED {
		t.Fatalf("terminal phase = %v; want FAILED", last.Phase)
	}
	if last.ErrorCode != "version_mismatch" {
		t.Fatalf("error_code = %q; want version_mismatch", last.ErrorCode)
	}
}

func TestUpgradeRunner_BadSignatureRefuses(t *testing.T) {
	fd, pub, _ := buildFakeRelease(t, "1.6.0")
	// Tamper: flip a bit in the signature.
	fd.signature = []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa==")

	progress, _, exitCode, err := runUpgrade(t, fd, pub, &v2pb.AgentUpgradeRequest{Channel: "stable"})
	if err == nil {
		t.Fatalf("Handle: expected error on bad signature")
	}
	if exitCode != nil {
		t.Fatalf("ExitFn should NOT have been called on bad sig; got %v", *exitCode)
	}
	last := progress[len(progress)-1]
	if last.Phase != v2pb.UpgradeProgress_PHASE_FAILED {
		t.Fatalf("terminal phase = %v; want FAILED", last.Phase)
	}
	if last.ErrorCode != "signature_mismatch" {
		t.Fatalf("error_code = %q; want signature_mismatch", last.ErrorCode)
	}
}

func TestUpgradeRunner_NoSigningKeyRefuses(t *testing.T) {
	fd, _, _ := buildFakeRelease(t, "1.6.0")

	progress, _, exitCode, err := func() ([]*v2pb.UpgradeProgress, string, *int, error) {
		// Replicate runUpgrade but with empty pubkey.
		srv := httptest.NewTLSServer(fd.handler())
		t.Cleanup(srv.Close)
		dir := t.TempDir()
		bin := filepath.Join(dir, "agent")
		_ = os.WriteFile(bin, []byte("OLD"), 0o755)
		var captured *int
		buf := &bytes.Buffer{}
		stream := &readWriteCloser{Writer: &threadSafeWriter{w: buf}, Reader: &bytes.Buffer{}}
		runner := &UpgradeRunner{
			DistributorBaseURL:  srv.URL,
			HTTPClient:          srv.Client(),
			SigningPublicKeyB64: "", // the gate we're testing
			BinaryPath:          bin,
			ExitFn:              func(c int) { captured = &c },
		}
		err := runner.Handle(context.Background(), stream, &v2pb.AgentUpgradeRequest{Channel: "stable"})
		return readAllProgress(t, buf, time.Now().Add(time.Second)), bin, captured, err
	}()

	if err == nil {
		t.Fatalf("Handle: expected error when SigningPublicKeyB64 is empty")
	}
	if exitCode != nil {
		t.Fatalf("ExitFn should NOT have been called; got %v", *exitCode)
	}
	if len(progress) == 0 {
		t.Fatalf("expected at least a PHASE_FAILED frame")
	}
	last := progress[len(progress)-1]
	if last.Phase != v2pb.UpgradeProgress_PHASE_FAILED {
		t.Fatalf("terminal phase = %v; want FAILED", last.Phase)
	}
	if last.ErrorCode != "not_configured" {
		t.Fatalf("error_code = %q; want not_configured", last.ErrorCode)
	}
}

func TestUpgradeRunner_ManifestFetchFailureSurfaces(t *testing.T) {
	fd, pub, _ := buildFakeRelease(t, "1.6.0")
	fd.manifestStatus = http.StatusServiceUnavailable

	progress, _, exitCode, err := runUpgrade(t, fd, pub, &v2pb.AgentUpgradeRequest{Channel: "stable"})
	if err == nil {
		t.Fatalf("Handle: expected error on manifest 503")
	}
	if exitCode != nil {
		t.Fatalf("ExitFn should not run on manifest fetch failure")
	}
	last := progress[len(progress)-1]
	if last.Phase != v2pb.UpgradeProgress_PHASE_FAILED {
		t.Fatalf("terminal phase = %v; want FAILED", last.Phase)
	}
	if last.ErrorCode != "manifest_fetch_failed" {
		t.Fatalf("error_code = %q; want manifest_fetch_failed", last.ErrorCode)
	}
}

func TestUpgradeRunner_PlatformUnsupportedRefuses(t *testing.T) {
	fd, pub, priv := buildFakeRelease(t, "1.6.0")
	// Rewrite the manifest with a single artifact for some other
	// platform that won't match the test runner.
	m := distributorManifest{
		Version: "1.6.0",
		Channel: "stable",
		Artifacts: []distributorArtifact{{
			OS:     "plan9", // never matches a real Go test runner
			Arch:   "amd64",
			Key:    "platypus-agent-plan9-amd64",
			Size:   1,
			SHA256: hex.EncodeToString(sha256.New().Sum(nil)),
		}},
	}
	mb, _ := json.Marshal(m)
	fd.manifest = mb
	fd.signature = []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, mb)))

	progress, _, exitCode, err := runUpgrade(t, fd, pub, &v2pb.AgentUpgradeRequest{Channel: "stable"})
	if err == nil {
		t.Fatalf("Handle: expected error on platform mismatch")
	}
	if exitCode != nil {
		t.Fatalf("ExitFn should not run when platform unsupported")
	}
	last := progress[len(progress)-1]
	if last.Phase != v2pb.UpgradeProgress_PHASE_FAILED {
		t.Fatalf("terminal phase = %v; want FAILED", last.Phase)
	}
	if last.ErrorCode != "platform_unsupported" {
		t.Fatalf("error_code = %q; want platform_unsupported", last.ErrorCode)
	}
}

// _ = tls is here so a future add of a TLS-cert assertion test
// already has the import without a churn-y diff.
var _ = tls.VersionTLS12
