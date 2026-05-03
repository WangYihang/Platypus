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

// host_kv_get / host_kv_put give plugins a tiny per-plugin scratch
// store on disk, scoped to <stateDir>/kv/<key>. Sufficient for caching
// last-seen state across invocations or keeping rolling counters; not
// a database. Backed by a plain file-per-key layout — atomic on rename,
// no compaction, no eviction.
//
// Capability gate: CapKV. The validKVKey check is what keeps a
// malicious plugin from escaping the per-plugin namespace via
// "../../etc/passwd".

func (pctx *pluginCtx) hostKVGet(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapKV] {
		returnEnvelope(p, stack, denied("kv"))
		return
	}
	key, err := readStringArg(p, stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_key: "+err.Error()))
		return
	}
	if !validKVKey(key) {
		returnEnvelope(p, stack, failed("invalid_key"))
		return
	}
	data, err := os.ReadFile(filepath.Join(pctx.stateDir, "kv", key))
	if errors.Is(err, os.ErrNotExist) {
		returnEnvelope(p, stack, okData(nil))
		return
	}
	if err != nil {
		returnEnvelope(p, stack, failed("read: "+err.Error()))
		return
	}
	returnEnvelope(p, stack, envelope{Ok: true, Data: rawJSONString(string(data))})
}

func (pctx *pluginCtx) hostKVPut(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapKV] {
		returnEnvelope(p, stack, denied("kv"))
		return
	}
	key, err := readStringArg(p, stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_key: "+err.Error()))
		return
	}
	val, err := readStringArg(p, stack[1])
	if err != nil {
		returnEnvelope(p, stack, failed("read_val: "+err.Error()))
		return
	}
	if !validKVKey(key) {
		returnEnvelope(p, stack, failed("invalid_key"))
		return
	}
	if int64(len(val)) > pctx.maxKVValueSize {
		returnEnvelope(p, stack, failed(fmt.Sprintf("value_too_large: max=%d", pctx.maxKVValueSize)))
		return
	}
	dir := filepath.Join(pctx.stateDir, "kv")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		returnEnvelope(p, stack, failed("mkdir: "+err.Error()))
		return
	}
	tmp := filepath.Join(dir, key+".tmp")
	if err := os.WriteFile(tmp, []byte(val), 0o600); err != nil {
		returnEnvelope(p, stack, failed("write: "+err.Error()))
		return
	}
	if err := os.Rename(tmp, filepath.Join(dir, key)); err != nil {
		_ = os.Remove(tmp)
		returnEnvelope(p, stack, failed("rename: "+err.Error()))
		return
	}
	returnEnvelope(p, stack, envelope{Ok: true})
}

// validKVKey rejects any key that would escape the per-plugin state
// dir or break the simple file-per-key storage. Allowed: 1-128 chars
// of [A-Za-z0-9_.-]; rejected: empty, traversal (".." / "/" / leading
// "."), too long.
func validKVKey(k string) bool {
	if k == "" || len(k) > 128 || k == "." || k == ".." || strings.HasPrefix(k, ".") {
		return false
	}
	for i := 0; i < len(k); i++ {
		c := k[i]
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '.' || c == '-'
		if !ok {
			return false
		}
	}
	return true
}
