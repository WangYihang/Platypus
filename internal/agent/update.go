package agent

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/utils/signing"
	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// SigningPublicKey holds the base64-encoded Ed25519 public key used to
// verify the release manifest. It is set at link time via:
//
//	go build -ldflags "-X github.com/WangYihang/Platypus/internal/agent.SigningPublicKey=<b64>"
//
// If empty, self-update is disabled — an unsigned release channel is
// worse than no self-update at all.
var SigningPublicKey = ""

// updateManifestArtifact mirrors internal/core.ManifestArtifact. Kept
// local so the agent package doesn't depend on internal/core.
type updateManifestArtifact struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Key    string `json:"key"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// updateManifest is the subset of the manifest we parse agent-side.
type updateManifest struct {
	Version    string                   `json:"version"`
	Channel    string                   `json:"channel"`
	ReleasedAt time.Time                `json:"released_at"`
	Artifacts  []updateManifestArtifact `json:"artifacts"`
}

const (
	updateHTTPTimeout = 5 * time.Minute
	manifestMaxBytes  = 1 << 20 // 1 MiB — manifests are small JSON
)

// performUpdate runs the full signed self-update flow. Returns a path
// to the verified replacement binary; caller re-execs it.
func performUpdate(req *agentpb.UpdateRequest) (string, error) {
	if SigningPublicKey == "" {
		return "", errors.New("update: no signing public key baked into this build; refusing to self-update")
	}
	pub, err := signing.DecodePublicKey(SigningPublicKey)
	if err != nil {
		return "", fmt.Errorf("update: decode embedded public key: %w", err)
	}

	base := strings.TrimRight(req.BaseUrl, "/")
	channel := req.Channel
	if channel == "" {
		channel = "stable"
	}

	client := &http.Client{Timeout: updateHTTPTimeout}

	manifestBytes, err := fetchAll(client, fmt.Sprintf("%s/v1/manifest/%s", base, channel), manifestMaxBytes)
	if err != nil {
		return "", fmt.Errorf("update: fetch manifest: %w", err)
	}
	sigBytes, err := fetchAll(client, fmt.Sprintf("%s/v1/manifest/%s/signature", base, channel), manifestMaxBytes)
	if err != nil {
		return "", fmt.Errorf("update: fetch manifest signature: %w", err)
	}
	if err := signing.Verify(pub, manifestBytes, sigBytes); err != nil {
		return "", fmt.Errorf("update: %w", err)
	}

	var m updateManifest
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		return "", fmt.Errorf("update: parse manifest: %w", err)
	}
	if m.Version != req.Version {
		return "", fmt.Errorf("update: manifest version %q != requested %q", m.Version, req.Version)
	}

	art := findArtifact(m.Artifacts, runtime.GOOS, runtime.GOARCH)
	if art == nil {
		return "", fmt.Errorf("update: manifest has no artifact for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	dlURL := fmt.Sprintf("%s/v1/artifacts/%s/%s/%s", base, runtime.GOOS, runtime.GOARCH, m.Version)
	tmp, gotSHA, err := downloadToTemp(client, dlURL, art.Size)
	if err != nil {
		return "", fmt.Errorf("update: download artifact: %w", err)
	}
	expected, err := hex.DecodeString(art.SHA256)
	if err != nil || !constEq(gotSHA, expected) {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("update: sha256 mismatch (manifest=%s got=%s)", art.SHA256, hex.EncodeToString(gotSHA))
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("update: chmod: %w", err)
	}
	return tmp, nil
}

// handleUpdate runs performUpdate and, on success, re-execs into the
// new binary. On any failure the current process keeps running.
func handleUpdate(_ *Client, req *agentpb.UpdateRequest) {
	log.Info("Self-update requested: version=%s channel=%s", req.Version, req.Channel)
	newPath, err := performUpdate(req)
	if err != nil {
		log.Error("Self-update aborted: %s", err)
		return
	}
	log.Success("Self-update to v%s verified, re-execing %s", req.Version, newPath)
	if err := syscall.Exec(newPath, []string{newPath}, os.Environ()); err != nil {
		log.Error("Self-update exec failed: %s", err)
	}
}

func fetchAll(client *http.Client, url string, maxBytes int64) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBytes))
}

// downloadToTemp streams a URL into a temp file while computing SHA-256
// on the fly. Returns the temp path and the digest.
func downloadToTemp(client *http.Client, url string, expectedSize int64) (string, []byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}

	f, err := os.CreateTemp("", "platypus-agent-*")
	if err != nil {
		return "", nil, err
	}
	tmp := f.Name()
	h := sha256.New()
	// Limit the stream to slightly more than expectedSize so a lying
	// Content-Length can't exhaust disk.
	var limit int64 = expectedSize + (1 << 20)
	if expectedSize <= 0 {
		limit = 256 << 20 // 256 MiB cap if manifest didn't specify
	}
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, limit))
	_ = f.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return "", nil, err
	}
	if expectedSize > 0 && n != expectedSize {
		_ = os.Remove(tmp)
		return "", nil, fmt.Errorf("short read: got %d bytes, manifest says %d", n, expectedSize)
	}
	return tmp, h.Sum(nil), nil
}

func findArtifact(arts []updateManifestArtifact, os, arch string) *updateManifestArtifact {
	for i := range arts {
		if arts[i].OS == os && arts[i].Arch == arch {
			return &arts[i]
		}
	}
	return nil
}

// constEq is a constant-time equality check. SHA-256 digests are not
// secret, but doing this anyway avoids teaching callers to sometimes
// reach for bytes.Equal on cryptographic comparisons.
func constEq(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// Compile-time assertion that ed25519 is linked in (we touch the
// package via signing.Verify; this keeps the dependency honest if
// someone refactors).
var _ = ed25519.PublicKeySize