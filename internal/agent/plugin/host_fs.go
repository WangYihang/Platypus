package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	extism "github.com/extism/go-sdk"
)

// host_fs_read / host_fs_listdir / host_fs_stat give plugins read-only
// filesystem access bounded to manifest.capabilities.fs.read.paths.
// Symlinks are resolved before the allowlist check so a symlink under
// an allowed dir pointing somewhere unallowed (e.g. /etc/nginx/foo ->
// /etc/shadow) is rejected.
//
// Capability gate: CapFSRead (plus per-path manifest spec).

// fsListEntry is the on-the-wire shape returned by host_fs_listdir
// (one per entry) and host_fs_stat (single).
type fsListEntry struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mtime_unix"`
}

func (pctx *pluginCtx) hostFSRead(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapFSRead] {
		returnEnvelope(p, stack, denied("fs.read"))
		return
	}
	path, err := readStringArg(p, stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_path: "+err.Error()))
		return
	}
	clean, err := pctx.checkFSReadPath(path)
	if err != nil {
		returnEnvelope(p, stack, failed(err.Error()))
		return
	}
	f, err := os.Open(clean)
	if err != nil {
		returnEnvelope(p, stack, failed("open: "+err.Error()))
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		returnEnvelope(p, stack, failed("stat: "+err.Error()))
		return
	}
	if st.IsDir() {
		returnEnvelope(p, stack, failed("is_directory"))
		return
	}
	if st.Size() > pctx.maxFileReadSize {
		returnEnvelope(p, stack, failed(fmt.Sprintf(
			"file_too_large: size=%d max=%d", st.Size(), pctx.maxFileReadSize)))
		return
	}
	// io.ReadAll bounded by maxFileReadSize+1 — a Stat()-reported
	// size is meaningless for /proc and /sys virtual files (they
	// report size=0 but stream real content on read). Capping at
	// maxFileReadSize+1 lets the per-plugin limit still bite for
	// genuinely large files while letting /proc/meminfo etc. flow
	// through. Allocates only what's read; no upfront pre-allocation
	// hurts reading large real files but the write-then-cap shape is
	// far less subtle than "trust st.Size()".
	limited := io.LimitReader(f, pctx.maxFileReadSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		returnEnvelope(p, stack, failed("read: "+err.Error()))
		return
	}
	if int64(len(data)) > pctx.maxFileReadSize {
		returnEnvelope(p, stack, failed(fmt.Sprintf(
			"file_too_large: read>%d", pctx.maxFileReadSize)))
		return
	}
	returnEnvelope(p, stack, envelope{Ok: true, Data: rawJSONString(string(data))})
}

// fsReadRangeRequest is the JSON the wasm passes when it wants a
// specific byte slice of a file rather than the whole thing. Used by
// streaming-style plugins (e.g. sys-file-read) that chunk a large
// file into wire-sized frames; the existing host_fs_read returns
// the whole file (capped by maxFileReadSize), which doesn't scale.
type fsReadRangeRequest struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset"`
	// Length is the max bytes to return. 0 = "all remaining bytes
	// from offset", clamped at maxFileReadRangeBytes per call so a
	// single call can't blow the wasm memory limit.
	Length int64 `json:"length"`
}

// fsReadRangeResponse is the structured envelope.Data the wasm
// receives. EOF is true when offset+returned bytes covered the
// trailing tail of the file (no more bytes available).
type fsReadRangeResponse struct {
	Data   []byte `json:"data"`
	EOF    bool   `json:"eof"`
	Size   int64  `json:"size"`
	Mode   uint32 `json:"mode"`
}

// maxFileReadRangeBytes caps a single host_fs_read_range call so a
// wasm asking for "Length=10 GiB" still returns sanely. Tuned to
// stay well under internal/link.FrameMaxBytes (1 MiB) so the
// returned bytes can be wrapped in a single wire frame on the
// downstream legacy bridge path.
const maxFileReadRangeBytes = 256 * 1024

// hostFSReadRange reads a specific byte slice of a regular file.
// Stateless on the host side: opens, seeks, reads, closes per call.
// Per-chunk overhead is a few syscalls — fine for the sequential
// streaming pattern this fn is intended for, where each chunk is
// 64-256 KiB. A handle-based API would amortise the syscalls but
// requires per-plugin state lifecycle that's not worth the
// complexity for the (rare-in-practice) high-throughput case.
func (pctx *pluginCtx) hostFSReadRange(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapFSRead] {
		returnEnvelope(p, stack, denied("fs.read"))
		return
	}
	raw, err := readStringArg(p, stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_arg: "+err.Error()))
		return
	}
	var req fsReadRangeRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		returnEnvelope(p, stack, failed("parse_request: "+err.Error()))
		return
	}
	clean, err := pctx.checkFSReadPath(req.Path)
	if err != nil {
		returnEnvelope(p, stack, failed(err.Error()))
		return
	}
	f, err := os.Open(clean)
	if err != nil {
		returnEnvelope(p, stack, failed("open: "+err.Error()))
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		returnEnvelope(p, stack, failed("stat: "+err.Error()))
		return
	}
	if st.IsDir() {
		returnEnvelope(p, stack, failed("is_directory"))
		return
	}
	want := req.Length
	if want <= 0 || want > maxFileReadRangeBytes {
		want = maxFileReadRangeBytes
	}
	if req.Offset > 0 {
		if _, err := f.Seek(req.Offset, io.SeekStart); err != nil {
			returnEnvelope(p, stack, failed("seek: "+err.Error()))
			return
		}
	}
	buf := make([]byte, want)
	n, err := io.ReadFull(f, buf)
	// io.ReadFull returns ErrUnexpectedEOF when it reads at least
	// one byte but fewer than len(buf) — that's our signal that we
	// hit the file's tail. EOF (zero bytes read) is the same signal
	// when offset already pointed past the end. Either way the
	// caller should know "no more after this batch".
	eof := false
	switch {
	case err == nil:
		// Buffer fully filled — there may be more data after.
	case err == io.EOF:
		eof = true
	case err == io.ErrUnexpectedEOF:
		eof = true
	default:
		returnEnvelope(p, stack, failed("read: "+err.Error()))
		return
	}
	resp := fsReadRangeResponse{
		Data: buf[:n],
		EOF:  eof,
		Size: st.Size(),
		Mode: uint32(st.Mode().Perm()),
	}
	returnEnvelope(p, stack, okData(resp))
}

func (pctx *pluginCtx) hostFSListdir(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapFSRead] {
		returnEnvelope(p, stack, denied("fs.read"))
		return
	}
	path, err := readStringArg(p, stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_path: "+err.Error()))
		return
	}
	clean, err := pctx.checkFSReadPath(path)
	if err != nil {
		returnEnvelope(p, stack, failed(err.Error()))
		return
	}
	entries, err := os.ReadDir(clean)
	if err != nil {
		returnEnvelope(p, stack, failed("readdir: "+err.Error()))
		return
	}
	out := make([]fsListEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, fsListEntry{
			Name: e.Name(), IsDir: e.IsDir(),
			Size: info.Size(), ModTime: info.ModTime().Unix(),
		})
	}
	returnEnvelope(p, stack, okData(out))
}

func (pctx *pluginCtx) hostFSStat(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapFSRead] {
		returnEnvelope(p, stack, denied("fs.read"))
		return
	}
	path, err := readStringArg(p, stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_path: "+err.Error()))
		return
	}
	clean, err := pctx.checkFSReadPath(path)
	if err != nil {
		returnEnvelope(p, stack, failed(err.Error()))
		return
	}
	st, err := os.Lstat(clean)
	if err != nil {
		returnEnvelope(p, stack, failed("stat: "+err.Error()))
		return
	}
	returnEnvelope(p, stack, okData(fsListEntry{
		Name: st.Name(), IsDir: st.IsDir(),
		Size: st.Size(), ModTime: st.ModTime().Unix(),
	}))
}

// checkFSReadPath resolves the requested path, verifies it does not
// escape the manifest's allowlist via symlinks or "..", and returns
// the cleaned absolute path. Symlinks are resolved eagerly so a
// symlink from /etc/nginx/foo -> /etc/shadow is rejected even though
// /etc/nginx is allowed.
func (pctx *pluginCtx) checkFSReadPath(path string) (string, error) {
	if pctx.manifest.Capabilities.FSRead == nil {
		return "", errors.New("capability_denied: fs.read (no manifest spec)")
	}
	if !filepath.IsAbs(path) {
		return "", errors.New("path_not_absolute")
	}
	clean := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		// Pre-resolution path failed to resolve (missing file is a
		// legitimate result); use the cleaned path for the allowlist
		// check so the plugin gets a "not found" instead of a misleading
		// "denied" when the file simply doesn't exist.
		resolved = clean
	}
	for _, allowed := range pctx.manifest.Capabilities.FSRead.Paths {
		allowedClean, _ := filepath.EvalSymlinks(allowed)
		if allowedClean == "" {
			allowedClean = filepath.Clean(allowed)
		}
		if pathHasPrefix(resolved, allowedClean) {
			return resolved, nil
		}
	}
	return "", errors.New("capability_denied: path_not_in_allowlist")
}

// pathHasPrefix reports whether `p` is `prefix` or descends from it.
// Component-aware (so "/etc/nginx2" is NOT under "/etc/nginx"). Root
// prefix "/" is special-cased: any absolute path descends from it,
// matching the everyday "give the plugin full read access" intent of
// fs.read.paths=["/"] (used by the system ListDir / Stat plugins).
func pathHasPrefix(p, prefix string) bool {
	if p == prefix {
		return true
	}
	if prefix == string(filepath.Separator) {
		// Root: every absolute path is under it. The general suffix
		// check below would reject "/tmp/foo" because rest[0]=='t' is
		// not the separator — that logic is right for "/etc" but
		// wrong for "/" because the separator is the prefix itself.
		return filepath.IsAbs(p)
	}
	if !strings.HasPrefix(p, prefix) {
		return false
	}
	rest := p[len(prefix):]
	return len(rest) > 0 && rest[0] == filepath.Separator
}
