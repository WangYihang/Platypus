package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	extism "github.com/extism/go-sdk"
)

// host_fs_write / mkdir / chmod / rename / delete give plugins
// destructive filesystem access bounded to
// manifest.capabilities.fs.write.paths. Same enforcement model as
// fs.read: capability gate (CapFSWrite) + per-path allowlist + eager
// symlink resolution before the prefix check.
//
// Capability gate: CapFSWrite. The granted set comes from the
// operator-confirmed install or the system-bundle's full grant.
//
// Wire conventions follow host_fs.go: each call takes a single JSON
// request, returns a JSON envelope. The Rust side speaks to all of
// these via #[host_fn("platypus")].

// fsWriteRequest is the JSON shape for write-class operations that
// only need a single target path: mkdir, chmod, delete. Rename
// has its own two-path shape below.
type fsWriteRequest struct {
	Path      string `json:"path"`
	Mode      uint32 `json:"mode,omitempty"`      // mkdir / chmod
	MakeDirs  bool   `json:"mkdirs,omitempty"`    // mkdir: also create missing parents
	Recursive bool   `json:"recursive,omitempty"` // delete: rmtree
	Data      string `json:"data,omitempty"`      // write: file contents (string for JSON; treat as bytes)
}

type fsRenameRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func (pctx *pluginCtx) hostFSWrite(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	pctx.runFSWriteCall(p, stack, "write", func(req fsWriteRequest, target string) envelope {
		if err := os.WriteFile(target, []byte(req.Data), 0o600); err != nil {
			return failed("write: " + err.Error())
		}
		return envelope{Ok: true}
	})
}

func (pctx *pluginCtx) hostFSMkdir(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	pctx.runFSWriteCall(p, stack, "mkdir", func(req fsWriteRequest, target string) envelope {
		mode := os.FileMode(req.Mode)
		if mode == 0 {
			mode = 0o755
		}
		var err error
		if req.MakeDirs {
			err = os.MkdirAll(target, mode)
		} else {
			err = os.Mkdir(target, mode)
		}
		if err != nil {
			return failed("mkdir: " + err.Error())
		}
		return envelope{Ok: true}
	})
}

func (pctx *pluginCtx) hostFSChmod(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	pctx.runFSWriteCall(p, stack, "chmod", func(req fsWriteRequest, target string) envelope {
		if err := os.Chmod(target, os.FileMode(req.Mode)); err != nil {
			return failed("chmod: " + err.Error())
		}
		return envelope{Ok: true}
	})
}

func (pctx *pluginCtx) hostFSDelete(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	pctx.runFSWriteCall(p, stack, "delete", func(req fsWriteRequest, target string) envelope {
		var err error
		if req.Recursive {
			err = os.RemoveAll(target)
		} else {
			err = os.Remove(target)
		}
		if err != nil {
			return failed("delete: " + err.Error())
		}
		return envelope{Ok: true}
	})
}

// hostFSRename is its own beast because both `from` and `to` must be
// inside the allowlist. Fail fast if either ends up outside; renaming
// `/etc/foo` → `/tmp/bar` is a way to exfiltrate a file out of the
// allowlist's reach if we only checked one side.
func (pctx *pluginCtx) hostFSRename(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapFSWrite] {
		returnEnvelope(p, stack, denied("fs.write"))
		return
	}
	raw, err := readStringArg(p, stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_request: "+err.Error()))
		return
	}
	var req fsRenameRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		returnEnvelope(p, stack, failed("decode_request: "+err.Error()))
		return
	}
	from, err := pctx.checkFSWritePath(req.From)
	if err != nil {
		returnEnvelope(p, stack, failed("from: "+err.Error()))
		return
	}
	to, err := pctx.checkFSWritePath(req.To)
	if err != nil {
		returnEnvelope(p, stack, failed("to: "+err.Error()))
		return
	}
	if err := os.Rename(from, to); err != nil {
		returnEnvelope(p, stack, failed("rename: "+err.Error()))
		return
	}
	returnEnvelope(p, stack, envelope{Ok: true})
}

// runFSWriteCall is the boilerplate-saver for the three single-path
// write ops: capability check → request decode → path allowlist
// check → run the operation. Centralised so each per-op function
// stays focused on the actual filesystem call.
func (pctx *pluginCtx) runFSWriteCall(p *extism.CurrentPlugin, stack []uint64, op string,
	apply func(req fsWriteRequest, resolvedPath string) envelope) {
	if !pctx.granted[CapFSWrite] {
		returnEnvelope(p, stack, denied("fs.write"))
		return
	}
	raw, err := readStringArg(p, stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_request: "+err.Error()))
		return
	}
	var req fsWriteRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		returnEnvelope(p, stack, failed("decode_request: "+err.Error()))
		return
	}
	target, err := pctx.checkFSWritePath(req.Path)
	if err != nil {
		returnEnvelope(p, stack, failed(err.Error()))
		return
	}
	returnEnvelope(p, stack, apply(req, target))
	_ = op // reserved for per-op audit logging when we wire it
	_ = fmt.Sprintf
}

// checkFSWritePath mirrors checkFSReadPath but against the fs.write
// allowlist. Resolves symlinks eagerly so a symlink under an allowed
// dir pointing outside it cannot be used as a write portal. For
// not-yet-existing targets (mkdir, write to a new file), the parent
// dir is what we resolve — the new entry inherits the parent's
// allowlist position.
func (pctx *pluginCtx) checkFSWritePath(path string) (string, error) {
	if pctx.manifest.Capabilities.FSWrite == nil {
		return "", errors.New("capability_denied: fs.write (no manifest spec)")
	}
	if !filepath.IsAbs(path) {
		return "", errors.New("path_not_absolute")
	}
	clean := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		// Path doesn't exist yet. Resolve the parent dir + retain the
		// final component so the write goes where the operator expects
		// (otherwise mkdir / write-new-file would always fail).
		parent, err := filepath.EvalSymlinks(filepath.Dir(clean))
		if err != nil {
			parent = filepath.Dir(clean)
		}
		resolved = filepath.Join(parent, filepath.Base(clean))
	}
	for _, allowed := range pctx.manifest.Capabilities.FSWrite.Paths {
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
