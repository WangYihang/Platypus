package plugin

import (
	"context"
	"errors"
	"fmt"
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
	data := make([]byte, st.Size())
	n, err := f.Read(data)
	if err != nil && int64(n) != st.Size() {
		returnEnvelope(p, stack, failed("read: "+err.Error()))
		return
	}
	returnEnvelope(p, stack, envelope{Ok: true, Data: rawJSONString(string(data[:n]))})
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
