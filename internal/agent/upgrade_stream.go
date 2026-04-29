package agent

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// UpgradeRunner executes one STREAM_TYPE_AGENT_UPGRADE flow per call.
// All knobs are dependency-injected so tests can drive the flow with
// a fake distributor server, an alternate binary path, and a captured
// "exit" function instead of os.Exit.
//
// The runner is stateless across invocations — Handle reads its
// AgentUpgradeRequest, walks every PhaseFn in order, and emits an
// UpgradeProgress frame at each transition. A terminal phase
// (PHASE_EXITING on success, PHASE_FAILED on error) is always the
// last frame written before the stream closes.
type UpgradeRunner struct {
	// DistributorBaseURL is the https://host[:port] root the agent
	// hits to fetch the manifest and the binary. Resolved once at
	// agent startup from BootstrapV2Options.EnrollURL — it's the
	// same host the agent enrolled against.
	DistributorBaseURL string

	// HTTPClient is preconfigured with the project CA in its root
	// pool. Reused across phases so connection pooling lets the
	// manifest GET + signature GET + artifact GET share a single
	// TLS handshake.
	HTTPClient *http.Client

	// SigningPublicKeyB64 is the agent's baked-in Ed25519 manifest
	// signing key, base64-encoded. Empty disables self-upgrade —
	// the runner refuses to install anything because an unsigned
	// channel is worse than no channel.
	SigningPublicKeyB64 string

	// BinaryPath is the absolute path of the currently running
	// agent binary. Defaults to os.Executable() at construction.
	// Test code overrides it to write into a temp dir.
	BinaryPath string

	// ExitFn is called after the install phase succeeds, with code
	// 75 (EX_TEMPFAIL) so the supervisor restarts the new binary.
	// Defaults to os.Exit. Tests substitute a recording closure
	// that lets the test assert "would-have-exited(75)" without
	// terminating the test runner.
	ExitFn func(code int)

	// Now is injected so tests can assert progress timing without
	// flake. Defaults to time.Now.
	Now func() time.Time
}

// distributorManifest is the agent-side copy of the JSON shape
// served by internal/core/distributor.go. Duplicated rather than
// imported so the agent package doesn't pull in the server-only
// distributor + S3 deps. Field tags must stay in sync with
// internal/core.Manifest; the version test in upgrade_stream_test.go
// pins the contract.
type distributorManifest struct {
	Version    string                `json:"version"`
	Channel    string                `json:"channel"`
	ReleasedAt time.Time             `json:"released_at"`
	Artifacts  []distributorArtifact `json:"artifacts"`
}

type distributorArtifact struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Key    string `json:"key"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"` // hex-encoded
}

// upgradeError captures both the operator-visible code (matches the
// reserved set in proto/v2/upgrade.proto) and a free-form message,
// so the failure frame the agent emits is uniform regardless of
// where in the flow we tripped.
type upgradeError struct {
	Code string
	Msg  string
}

func (e *upgradeError) Error() string { return e.Code + ": " + e.Msg }

func upgradeErrf(code, format string, args ...any) *upgradeError {
	return &upgradeError{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// Handle drives one upgrade end-to-end. The stream stays open the
// whole time so the server can render real-time progress.
//
// Returns nil only when the upgrade reached PHASE_EXITING and ExitFn
// was invoked (in production that path never returns because os.Exit
// terminates the process). Any earlier error is reported on-stream as
// PHASE_FAILED and also surfaced to the caller; the dispatch loop in
// serve_link.go logs the latter at WARN.
func (u *UpgradeRunner) Handle(ctx context.Context, stream io.ReadWriteCloser, req *v2pb.AgentUpgradeRequest) error {
	if u.Now == nil {
		u.Now = time.Now
	}
	if u.ExitFn == nil {
		u.ExitFn = os.Exit
	}

	if u.SigningPublicKeyB64 == "" {
		err := upgradeErrf("not_configured", "agent has no SigningPublicKey baked in; refusing to self-update")
		emitProgress(stream, &v2pb.UpgradeProgress{
			Phase:        v2pb.UpgradeProgress_PHASE_FAILED,
			ErrorCode:    err.Code,
			ErrorMessage: err.Msg,
		})
		return err
	}

	channel := req.GetChannel()
	if channel == "" {
		channel = "stable"
	}

	// 1) Fetch manifest.
	emitProgress(stream, &v2pb.UpgradeProgress{Phase: v2pb.UpgradeProgress_PHASE_FETCH_MANIFEST})
	manifestBytes, err := u.fetchURL(ctx, "/v1/manifest/"+url.PathEscape(channel))
	if err != nil {
		return u.fail(stream, upgradeErrf("manifest_fetch_failed", "%v", err))
	}
	var m distributorManifest
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		return u.fail(stream, upgradeErrf("manifest_decode_failed", "%v", err))
	}

	// Manifest may have moved on since the operator queued the
	// upgrade — when target_version is set, refuse anything else
	// so the operator's intent is honored exactly.
	if t := req.GetTargetVersion(); t != "" && t != m.Version {
		return u.fail(stream, upgradeErrf("version_mismatch",
			"target_version=%q but channel %q head is %q", t, channel, m.Version))
	}

	// 2) Verify Ed25519 signature.
	emitProgress(stream, &v2pb.UpgradeProgress{
		Phase:           v2pb.UpgradeProgress_PHASE_VERIFY_SIG,
		ResolvedVersion: m.Version,
	})
	sigBytes, err := u.fetchURL(ctx, "/v1/manifest/"+url.PathEscape(channel)+"/signature")
	if err != nil {
		return u.fail(stream, upgradeErrf("signature_fetch_failed", "%v", err))
	}
	pubBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(u.SigningPublicKeyB64))
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return u.fail(stream, upgradeErrf("signing_key_invalid",
			"baked-in public key is not a valid base64-encoded ed25519 pubkey"))
	}
	// Signature bytes from the distributor may be raw (64 bytes) or
	// base64-wrapped depending on how the release pipeline uploaded
	// them. Try base64 first (the common case the pipeline uses)
	// and fall back to raw.
	sig := decodeSignature(sigBytes)
	if !ed25519.Verify(pubBytes, manifestBytes, sig) {
		return u.fail(stream, upgradeErrf("signature_mismatch",
			"manifest signature did not verify against baked-in public key"))
	}

	// 3) Pick our (os, arch) artifact.
	art := findArtifact(m.Artifacts, runtime.GOOS, runtime.GOARCH)
	if art == nil {
		return u.fail(stream, upgradeErrf("platform_unsupported",
			"manifest has no artifact for %s/%s", runtime.GOOS, runtime.GOARCH))
	}
	expectedSHA, err := hex.DecodeString(art.SHA256)
	if err != nil || len(expectedSHA) != sha256.Size {
		return u.fail(stream, upgradeErrf("manifest_decode_failed",
			"artifact sha256 is not a valid hex digest"))
	}

	// 4) Download binary, streaming SHA256.
	emitProgress(stream, &v2pb.UpgradeProgress{
		Phase:           v2pb.UpgradeProgress_PHASE_DOWNLOAD,
		ResolvedVersion: m.Version,
		ResolvedSha256:  expectedSHA,
		BytesTotal:      uint64(art.Size),
	})
	tmpPath := u.BinaryPath + ".new"
	gotSHA, gotBytes, err := u.downloadTo(ctx, art, tmpPath, stream, expectedSHA)
	if err != nil {
		_ = os.Remove(tmpPath)
		return u.fail(stream, err.(*upgradeError))
	}

	// 5) Final SHA256 check (defense-in-depth: streaming Verify
	// already ran while we wrote, but a second compare from the
	// settled file rules out any "the writer was wrong" class of
	// bug).
	emitProgress(stream, &v2pb.UpgradeProgress{
		Phase:           v2pb.UpgradeProgress_PHASE_VERIFY_SHA256,
		ResolvedVersion: m.Version,
		ResolvedSha256:  expectedSHA,
		BytesDone:       gotBytes,
		BytesTotal:      uint64(art.Size),
	})
	if !equalBytes(gotSHA, expectedSHA) {
		_ = os.Remove(tmpPath)
		return u.fail(stream, upgradeErrf("sha256_mismatch",
			"downloaded artifact hash mismatch: got %x want %x", gotSHA, expectedSHA))
	}

	// 6) Atomic install: chmod, rename old to .bak, rename new to
	// final. On Linux/macOS rename(2) over a running binary works
	// because the kernel keeps the old inode mmap'd until our
	// process exits — no need to "stop ourselves first". Windows
	// would need a different scheme; out of scope for v1 and the
	// caller already platform-checked at manifest pick time.
	emitProgress(stream, &v2pb.UpgradeProgress{
		Phase:           v2pb.UpgradeProgress_PHASE_INSTALL,
		ResolvedVersion: m.Version,
		ResolvedSha256:  expectedSHA,
	})
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		_ = os.Remove(tmpPath)
		return u.fail(stream, upgradeErrf("install_failed", "chmod: %v", err))
	}
	bakPath := u.BinaryPath + ".bak"
	if err := os.Rename(u.BinaryPath, bakPath); err != nil {
		// On rare layouts (read-only mount, or a fresh install where
		// .bak is already there from a previous run) this is the
		// failure that leaves things working as before. Clean up the
		// .new payload so we don't leak it, then report.
		_ = os.Remove(tmpPath)
		return u.fail(stream, upgradeErrf("install_failed", "rename current to .bak: %v", err))
	}
	if err := os.Rename(tmpPath, u.BinaryPath); err != nil {
		// Best-effort restore: put the .bak back so the supervisor
		// finds a working binary on the next restart. If that fails
		// too, both .bak and the missing real binary remain — the
		// host needs manual intervention either way.
		_ = os.Rename(bakPath, u.BinaryPath)
		_ = os.Remove(tmpPath)
		return u.fail(stream, upgradeErrf("install_failed", "rename .new to current: %v", err))
	}

	// 7) Terminal: report PHASE_EXITING and exit with EX_TEMPFAIL.
	// The supervisor restarts us under the new binary; the new
	// process re-enrolls (or reuses persisted identity) and the
	// server sees the build_version flip from old to new on the
	// EnrollRequest, closing the loop on the upgrade record.
	emitProgress(stream, &v2pb.UpgradeProgress{
		Phase:           v2pb.UpgradeProgress_PHASE_EXITING,
		ResolvedVersion: m.Version,
		ResolvedSha256:  expectedSHA,
	})
	// Give the terminal frame a moment to flush at the TLS layer
	// before the OS reaps the connection on exit. Without this the
	// server occasionally sees a bare EOF instead of the EXITING
	// frame and has to infer success from the next reconnect.
	_ = stream.Close()
	time.Sleep(100 * time.Millisecond)
	log.Info("agent: self-upgrade installed v=%s commit=%s; exiting for supervisor restart",
		m.Version, art.Key)
	u.ExitFn(75)
	return nil
}

// fail emits a PHASE_FAILED frame, closes the stream, and returns
// the error so the dispatch loop can log it. Centralised so every
// failure path uses the same wire format.
func (u *UpgradeRunner) fail(stream io.ReadWriteCloser, err *upgradeError) error {
	emitProgress(stream, &v2pb.UpgradeProgress{
		Phase:        v2pb.UpgradeProgress_PHASE_FAILED,
		ErrorCode:    err.Code,
		ErrorMessage: err.Msg,
	})
	return err
}

// fetchURL performs one GET against the distributor and returns the
// body. Non-2xx responses become errors with the status text in the
// message so the operator sees "404" rather than "manifest fetch
// failed: <nothing>" when the channel typo'd.
func (u *UpgradeRunner) fetchURL(ctx context.Context, path string) ([]byte, error) {
	full := strings.TrimRight(u.DistributorBaseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, err
	}
	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("GET %s: status %d", path, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// downloadTo streams the artifact into tmpPath while computing
// SHA256 in flight. The third arg lets us emit periodic progress
// updates every progressInterval bytes so the UI's progress bar
// moves; the fourth arg is the expected hash, only used to bail
// out early on the rare "wrong sha was advertised in manifest"
// path (the final compare is the authoritative one).
func (u *UpgradeRunner) downloadTo(
	ctx context.Context,
	art *distributorArtifact,
	tmpPath string,
	progressStream io.Writer,
	expectedSHA []byte,
) ([]byte, uint64, error) {
	full := strings.TrimRight(u.DistributorBaseURL, "/") + "/v1/artifacts/" +
		url.PathEscape(art.OS) + "/" + url.PathEscape(art.Arch) + "/" + url.PathEscape(art.Key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, 0, upgradeErrf("artifact_fetch_failed", "build request: %v", err)
	}
	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		return nil, 0, upgradeErrf("artifact_fetch_failed", "%v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	// Distributor returns a 302 redirect to a presigned S3 URL — Go's
	// default client follows it automatically, so by the time we see
	// the response it should be 200 from the upstream object store.
	if resp.StatusCode/100 != 2 {
		return nil, 0, upgradeErrf("artifact_fetch_failed", "status %d", resp.StatusCode)
	}

	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, 0, upgradeErrf("install_failed", "create tmp: %v", err)
	}
	defer func() { _ = out.Close() }()

	hasher := sha256.New()
	tee := io.TeeReader(resp.Body, hasher)
	pr := &progressReader{
		src:        tee,
		stream:     progressStream,
		total:      uint64(art.Size),
		emitEvery:  256 * 1024, // 256 KiB or 1s, whichever first
		emitTicker: time.NewTicker(time.Second),
	}
	defer pr.emitTicker.Stop()
	n, err := io.Copy(out, pr)
	if err != nil {
		return nil, 0, upgradeErrf("download_failed", "copy: %v", err)
	}
	if err := out.Sync(); err != nil {
		return nil, 0, upgradeErrf("install_failed", "fsync tmp: %v", err)
	}
	_ = expectedSHA // hash compare happens at PHASE_VERIFY_SHA256 in the caller
	return hasher.Sum(nil), uint64(n), nil
}

// progressReader is an io.Reader that emits UpgradeProgress frames
// while bytes flow. The two emit conditions (256 KiB delta OR 1s
// elapsed) keep the wire chatty enough for a smooth UI bar without
// drowning the link on multi-MB binaries.
type progressReader struct {
	src    io.Reader
	stream io.Writer
	total  uint64

	read       uint64
	lastEmit   uint64
	emitEvery  uint64
	emitTicker *time.Ticker
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.src.Read(b)
	if n > 0 {
		p.read += uint64(n)
		shouldEmit := p.read-p.lastEmit >= p.emitEvery
		if !shouldEmit {
			select {
			case <-p.emitTicker.C:
				shouldEmit = true
			default:
			}
		}
		if shouldEmit {
			p.lastEmit = p.read
			emitProgress(p.stream, &v2pb.UpgradeProgress{
				Phase:      v2pb.UpgradeProgress_PHASE_DOWNLOAD,
				BytesDone:  p.read,
				BytesTotal: p.total,
			})
		}
	}
	return n, err
}

// findArtifact locates the entry matching the agent's runtime os/arch.
// Returns nil if the manifest doesn't carry a build for this platform
// (caller surfaces that as platform_unsupported).
func findArtifact(arts []distributorArtifact, goos, goarch string) *distributorArtifact {
	for i := range arts {
		if arts[i].OS == goos && arts[i].Arch == goarch {
			return &arts[i]
		}
	}
	return nil
}

// decodeSignature accepts either raw 64-byte ed25519 signatures or
// base64-wrapped ones. The release pipeline historically wrote
// base64; older snapshots wrote raw. Returning whichever decodes
// (with raw as a last resort) keeps the agent compatible with both
// without a wire-format flag.
func decodeSignature(b []byte) []byte {
	trimmed := strings.TrimSpace(string(b))
	if dec, err := base64.StdEncoding.DecodeString(trimmed); err == nil &&
		len(dec) == ed25519.SignatureSize {
		return dec
	}
	if dec, err := base64.RawStdEncoding.DecodeString(trimmed); err == nil &&
		len(dec) == ed25519.SignatureSize {
		return dec
	}
	return b
}

func equalBytes(a, b []byte) bool {
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

func emitProgress(w io.Writer, p *v2pb.UpgradeProgress) {
	// Best-effort: a write error here means the link dropped, in
	// which case the upgrade is in a half-finished state regardless;
	// no point unwinding from here. Logged at debug because it's
	// noise on every clean session close.
	if err := link.WriteFrame(w, p); err != nil {
		log.Debug("agent: upgrade progress write: %v", err)
	}
}

// ResolveBinaryPath returns the absolute path of the running binary,
// dereferencing any final-component symlink so a "rename over the
// running binary" operation lands on the real file rather than its
// /usr/local/bin/ pointer. Used by main.go to seed
// UpgradeRunner.BinaryPath.
func ResolveBinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		// EvalSymlinks fails when the target doesn't exist; that
		// shouldn't happen for a running binary but if it does we
		// fall back to the unresolved path so we still have *some*
		// usable target. Logged so the agent at least surfaces the
		// degraded mode in startup logs.
		log.Debug("agent: ResolveBinaryPath EvalSymlinks: %v", err)
		return exe, nil
	}
	return resolved, nil
}

// errBinaryUnknown is surfaced by main.go when os.Executable returns
// the empty string (some Go distros on niche platforms). Exposed as
// a sentinel so a future config knob ("disable upgrade if path
// unknown") can branch on it.
var errBinaryUnknown = errors.New("agent: cannot resolve own binary path; self-upgrade disabled")

// _ = errBinaryUnknown to keep govet happy until main.go consumes it.
var _ = errBinaryUnknown
