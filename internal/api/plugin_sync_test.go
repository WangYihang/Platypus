package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// fakeSession captures every Open call and lets the test script the
// agent-side responses for each. Avoids standing up yamux + WS.
type fakeSession struct {
	// installedVersions is the {id: version} map returned for the
	// first PluginMgmt:list. Empty version means "not installed".
	installedVersions map[string]string
	// installScript maps "plugin_id" → terminal phase to emit on the
	// install stream. Missing entries default to PHASE_INSTALLED.
	installScript map[string]v2pb.PluginInstallProgress_Phase
	// calls records every Open invocation in order so tests can assert
	// the exact agent-side conversation.
	calls []openCall
}

type openCall struct {
	streamType    v2pb.StreamType
	correlationID string
	pluginID      string
	op            string // "list" / "install"
}

func (f *fakeSession) Open(t v2pb.StreamType, metadata []byte, correlationID string) (io.ReadWriteCloser, error) {
	var req v2pb.PluginMgmtRequest
	if err := proto.Unmarshal(metadata, &req); err != nil {
		return nil, err
	}
	call := openCall{streamType: t, correlationID: correlationID}
	switch op := req.GetOp().(type) {
	case *v2pb.PluginMgmtRequest_List:
		call.op = "list"
		f.calls = append(f.calls, call)
		return f.serveList(), nil
	case *v2pb.PluginMgmtRequest_Install:
		call.op = "install"
		call.pluginID = op.Install.GetPluginId()
		f.calls = append(f.calls, call)
		return f.serveInstall(op.Install.GetPluginId()), nil
	default:
		return nil, errors.New("fake session: unsupported op")
	}
}

// serveList builds a (server) <-> (test) net.Pipe and writes a list
// response with the configured installed plugin ids. The reconciler
// reads from the server side via the returned ReadWriteCloser.
func (f *fakeSession) serveList() io.ReadWriteCloser {
	a, b := net.Pipe()
	infos := make([]*v2pb.PluginInfo, 0, len(f.installedVersions))
	for id, ver := range f.installedVersions {
		infos = append(infos, &v2pb.PluginInfo{Id: id, Version: ver, Enabled: true})
	}
	resp := &v2pb.PluginMgmtResponse{
		Result: &v2pb.PluginMgmtResponse_List{List: &v2pb.PluginListResponse{Plugins: infos}},
	}
	go func() {
		_ = link.WriteFrame(b, resp)
		// Don't close b yet — the reconciler will Close a, which the
		// pipe propagates as EOF on the b side.
	}()
	return a
}

// serveInstall drains the three install chunks the reconciler pushes
// and writes back a single PluginInstallProgress frame with the
// configured terminal phase.
func (f *fakeSession) serveInstall(pluginID string) io.ReadWriteCloser {
	a, b := net.Pipe()
	phase := v2pb.PluginInstallProgress_PHASE_INSTALLED
	if p, ok := f.installScript[pluginID]; ok {
		phase = p
	}
	go func() {
		// Drain the three install chunks the pusher writes
		// (manifest + wasm + sig).
		for i := 0; i < 3; i++ {
			var c v2pb.PluginInstallChunk
			if err := link.ReadFrame(b, &c); err != nil {
				return
			}
		}
		_ = link.WriteFrame(b, &v2pb.PluginInstallProgress{Phase: phase})
	}()
	return a
}

// stageBundle creates a minimal system-plugins/ tree with one plugin
// staged at the given (id, version). The wasm + sig are random bytes
// (the fake session never verifies them — that's the agent's job in
// production).
//
// The capabilities arg is unused by the manifest (the test fakes the
// agent side, so capability shapes don't matter), but the test does
// pass it through to assert the correlation_id used by the install
// stream.
func stageBundle(t *testing.T, root, pluginID, version string, _capabilities []string) {
	t.Helper()
	dir := filepath.Join(root, pluginID, version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Minimal valid manifest: any single rpc + sysinfo capability is
	// enough to satisfy ParseManifest's "at least one rpc or stream"
	// check. Resources are required + bounded.
	manifest := `api_version: 1
id: ` + pluginID + `
name: ` + pluginID + `
version: ` + version + `
author:
  name: test
license: MIT
runtime:
  type: wasm
  entry: plugin.wasm
  abi: extism/1
rpc:
  - name: ping
capabilities:
  sysinfo: true
resources:
  max_memory_mb: 16
  max_invocation_ms: 1000
signature:
  algo: minisign-ed25519
  key_id: TESTKEY
  sig_file: plugin.wasm.minisig
`
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	wasm := make([]byte, 16)
	_, _ = rand.Read(wasm)
	if err := os.WriteFile(filepath.Join(dir, "plugin.wasm"), wasm, 0o644); err != nil {
		t.Fatalf("write wasm: %v", err)
	}
	sig := []byte("untrusted comment: fake\n" + base64.StdEncoding.EncodeToString(wasm) + "\n")
	if err := os.WriteFile(filepath.Join(dir, "plugin.wasm.minisig"), sig, 0o644); err != nil {
		t.Fatalf("write sig: %v", err)
	}
}

// stagePublisherKey writes a non-empty publisher.pub at root. The
// fake session doesn't validate it; this is just so the reconciler's
// "trust anchor present" check passes.
func stagePublisherKey(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "publisher.pub"),
		[]byte("untrusted comment: fake\nKEY\n"), 0o644); err != nil {
		t.Fatalf("write publisher.pub: %v", err)
	}
}

func TestReconcileSystemPlugins_MandatoryCoreOnly(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "system-plugins")
	stagePublisherKey(t, root)
	stageBundle(t, root, "com.platypus.sys-info", "2.0.0", []string{"sysinfo"})

	sess := &fakeSession{installedVersions: nil}
	if err := reconcileSystemPlugins(context.Background(), sess, "agent-1", nil, dataDir); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	// Expect: 1 list call + 1 install call for sys-info.
	if len(sess.calls) != 2 {
		t.Fatalf("calls = %d (%+v); want 2", len(sess.calls), sess.calls)
	}
	if sess.calls[0].op != "list" {
		t.Errorf("first call op = %q; want list", sess.calls[0].op)
	}
	if sess.calls[1].op != "install" || sess.calls[1].pluginID != "com.platypus.sys-info" {
		t.Errorf("second call = %+v; want install of sys-info", sess.calls[1])
	}
}

func TestReconcileSystemPlugins_BaselinePicksPlusCore(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "system-plugins")
	stagePublisherKey(t, root)
	stageBundle(t, root, "com.platypus.sys-info", "2.0.0", []string{"sysinfo"})
	stageBundle(t, root, "com.platypus.sys-files-read", "1.0.0", []string{"fs.read"})
	stageBundle(t, root, "com.platypus.sys-process", "1.0.0", []string{"exec", "process"})

	sess := &fakeSession{installedVersions: nil}
	baseline := []string{"com.platypus.sys-files-read", "com.platypus.sys-process"}
	if err := reconcileSystemPlugins(context.Background(), sess, "agent-1", baseline, dataDir); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	// list + 3 installs (baseline 2 + mandatory 1).
	if len(sess.calls) != 4 {
		t.Fatalf("calls = %d (%+v); want 4", len(sess.calls), sess.calls)
	}
	want := map[string]bool{
		"com.platypus.sys-files-read": false,
		"com.platypus.sys-process":    false,
		"com.platypus.sys-info":       false,
	}
	for _, c := range sess.calls[1:] {
		if c.op != "install" {
			t.Errorf("unexpected non-install call: %+v", c)
			continue
		}
		if _, ok := want[c.pluginID]; !ok {
			t.Errorf("install of unexpected plugin: %s", c.pluginID)
			continue
		}
		want[c.pluginID] = true
	}
	for id, seen := range want {
		if !seen {
			t.Errorf("missing install for %s", id)
		}
	}
}

func TestReconcileSystemPlugins_AlreadyInstalledIsNoOp(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "system-plugins")
	stagePublisherKey(t, root)
	stageBundle(t, root, "com.platypus.sys-info", "2.0.0", []string{"sysinfo"})

	sess := &fakeSession{installedVersions: map[string]string{"com.platypus.sys-info": "2.0.0"}}
	if err := reconcileSystemPlugins(context.Background(), sess, "agent-1",
		[]string{"com.platypus.sys-info"}, dataDir); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	// list only; nothing missing.
	if len(sess.calls) != 1 || sess.calls[0].op != "list" {
		t.Fatalf("calls = %+v; want only a list", sess.calls)
	}
}

func TestReconcileSystemPlugins_VersionMismatchTriggersUpgrade(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "system-plugins")
	stagePublisherKey(t, root)
	// Stage v1.0.1 as the latest; agent claims to have v1.0.0.
	stageBundle(t, root, "com.platypus.sys-info", "1.0.1", []string{"sysinfo"})

	sess := &fakeSession{installedVersions: map[string]string{
		"com.platypus.sys-info": "1.0.0",
	}}
	if err := reconcileSystemPlugins(context.Background(), sess, "agent-1",
		[]string{"com.platypus.sys-info"}, dataDir); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	// list + install (the upgrade).
	if len(sess.calls) != 2 {
		t.Fatalf("calls = %+v; want list + install", sess.calls)
	}
	if sess.calls[1].op != "install" || sess.calls[1].pluginID != "com.platypus.sys-info" {
		t.Errorf("second call = %+v; want install of sys-info", sess.calls[1])
	}
}

func TestReconcileSystemPlugins_MissingFromBundleIsLoggedNotFatal(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "system-plugins")
	stagePublisherKey(t, root)
	// Stage only sys-info; baseline asks for one we don't have.
	stageBundle(t, root, "com.platypus.sys-info", "2.0.0", []string{"sysinfo"})

	sess := &fakeSession{installedVersions: nil}
	baseline := []string{"com.platypus.sys-files-read"} // not staged
	if err := reconcileSystemPlugins(context.Background(), sess, "agent-1", baseline, dataDir); err != nil {
		t.Fatalf("reconcile should not error on missing bundle entry: %v", err)
	}
	// list + sys-info install only; missing baseline entry is skipped.
	if len(sess.calls) != 2 {
		t.Fatalf("calls = %+v; want 2 (list + sys-info)", sess.calls)
	}
	if sess.calls[1].pluginID != "com.platypus.sys-info" {
		t.Errorf("unexpected install %+v; want sys-info only", sess.calls[1])
	}
}

func TestReconcileSystemPlugins_NoBundleDirShortCircuits(t *testing.T) {
	sess := &fakeSession{}
	if err := reconcileSystemPlugins(context.Background(), sess, "agent-1",
		[]string{"com.platypus.sys-info"}, ""); err != nil {
		t.Fatalf("reconcile with empty bundle dir: %v", err)
	}
	if len(sess.calls) != 0 {
		t.Errorf("calls should be empty when systemBundleDir is empty, got %+v", sess.calls)
	}
}

func TestDedupeAppend(t *testing.T) {
	cases := []struct {
		name string
		base []string
		add  []string
		want []string
	}{
		{name: "empty + empty", base: nil, add: nil, want: nil},
		{name: "preserves order", base: nil, add: []string{"a", "b", "c"}, want: []string{"a", "b", "c"}},
		{name: "drops empty + dup",
			base: []string{"a"}, add: []string{"", "b", "a", "b", "c"},
			want: []string{"a", "b", "c"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dedupeAppend(append([]string{}, tc.base...), tc.add)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v; want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v; want %v", got, tc.want)
				}
			}
		})
	}
}
