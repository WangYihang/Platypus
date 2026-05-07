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
	"github.com/WangYihang/Platypus/internal/server/sysplugins"
	"github.com/WangYihang/Platypus/internal/storage"
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

// specsOf is a tiny test helper that wraps each plugin id into a
// minimal PluginSpec (empty caps, empty config, schema_version=0).
// Tests that exercise the rich-shape path (config_overrides /
// granted_capabilities / schema_version) construct PluginSpec
// literals directly.
func specsOf(ids ...string) []storage.PluginSpec {
	out := make([]storage.PluginSpec, 0, len(ids))
	for _, id := range ids {
		out = append(out, storage.PluginSpec{PluginID: id})
	}
	return out
}

type openCall struct {
	streamType    v2pb.StreamType
	correlationID string
	pluginID      string
	op            string // "list" / "install"
	// installConfigJSON / installCaps / installSchemaVersion capture
	// the rich-shape fields that PR 3.5 introduced on the
	// PluginInstallRequest. They're only populated for "install"
	// ops; tests that don't care about config leave them at the
	// zero value.
	installConfigJSON     []byte
	installCaps           []string
	installSchemaVersion  int32
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
		call.installConfigJSON = op.Install.GetConfigJson()
		call.installCaps = op.Install.GetGrantedCapabilities()
		call.installSchemaVersion = op.Install.GetConfigSchemaVersion()
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

// stageBundlePlatformed is the os_targets/arch_targets-aware sibling
// of stageBundle. Used by the OS-filter tests to check the
// reconciler skips plugins whose manifest's runtime.os_targets
// doesn't match the agent's reported runtime.GOOS.
func stageBundlePlatformed(t *testing.T, root, pluginID, version string, osTargets, archTargets []string) {
	t.Helper()
	dir := filepath.Join(root, pluginID, version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	osLine, archLine := "", ""
	if len(osTargets) > 0 {
		osLine = "  os_targets: [" + joinQuoted(osTargets) + "]\n"
	}
	if len(archTargets) > 0 {
		archLine = "  arch_targets: [" + joinQuoted(archTargets) + "]\n"
	}
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
` + osLine + archLine + `rpc:
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

func joinQuoted(xs []string) string {
	out := ""
	for i, s := range xs {
		if i > 0 {
			out += ", "
		}
		out += `"` + s + `"`
	}
	return out
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
	if err := reconcileSystemPlugins(context.Background(), sess, "agent-1", nil, "linux", "amd64", os.DirFS(filepath.Join(dataDir, "system-plugins"))); err != nil {
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
	baseline := specsOf("com.platypus.sys-files-read", "com.platypus.sys-process")
	if err := reconcileSystemPlugins(context.Background(), sess, "agent-1", baseline, "linux", "amd64", os.DirFS(filepath.Join(dataDir, "system-plugins"))); err != nil {
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
		specsOf("com.platypus.sys-info"), "linux", "amd64", os.DirFS(filepath.Join(dataDir, "system-plugins"))); err != nil {
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
		specsOf("com.platypus.sys-info"), "linux", "amd64", os.DirFS(filepath.Join(dataDir, "system-plugins"))); err != nil {
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
	baseline := specsOf("com.platypus.sys-files-read") // not staged
	if err := reconcileSystemPlugins(context.Background(), sess, "agent-1", baseline, "linux", "amd64", os.DirFS(filepath.Join(dataDir, "system-plugins"))); err != nil {
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

// TestReconcileSystemPlugins_PrebuiltEmbed exercises the actual
// internal/server/sysplugins.PrebuiltFS() against the reconciler.
// This is the end-to-end proof that the binary's embedded tree is
// usable: a fresh server install (no <data-dir>/system-plugins/
// override) goes through this exact path on every agent connect.
//
// Asserts the mandatory sys-info gets installed and no install
// errors propagate. We don't care about the other 7 plugins for
// this test — they ride along when an operator's baseline asks for
// them and are exercised by their own integration tests.
func TestReconcileSystemPlugins_PrebuiltEmbed(t *testing.T) {
	bundle := sysplugins.PrebuiltFS()
	sess := &fakeSession{installedVersions: nil}
	if err := reconcileSystemPlugins(context.Background(), sess, "agent-1", nil, "linux", "amd64", bundle); err != nil {
		t.Fatalf("reconcile against PrebuiltFS: %v", err)
	}
	// 1 list + 1 install (sys-info, the mandatory entry).
	if len(sess.calls) != 2 {
		t.Fatalf("calls = %d (%+v); want list + install of sys-info", len(sess.calls), sess.calls)
	}
	if sess.calls[1].op != "install" || sess.calls[1].pluginID != "com.platypus.sys-info" {
		t.Errorf("install call = %+v; want sys-info", sess.calls[1])
	}
}

func TestReconcileSystemPlugins_NoBundleDirShortCircuits(t *testing.T) {
	sess := &fakeSession{}
	if err := reconcileSystemPlugins(context.Background(), sess, "agent-1",
		specsOf("com.platypus.sys-info"), "linux", "amd64", nil); err != nil {
		t.Fatalf("reconcile with nil bundle: %v", err)
	}
	if len(sess.calls) != 0 {
		t.Errorf("calls should be empty when systemBundle is nil, got %+v", sess.calls)
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

// TestReconcileSystemPlugins_SkipsByOSTarget asserts a plugin whose
// manifest declares os_targets=[linux] is NOT pushed to a darwin
// agent, even though it's in the baseline.
func TestReconcileSystemPlugins_SkipsByOSTarget(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "system-plugins")
	stagePublisherKey(t, root)
	stageBundlePlatformed(t, root, "com.platypus.sys-info", "2.0.0", nil, nil) // mandatory, all platforms
	stageBundlePlatformed(t, root, "com.platypus.sys-systemd-linux", "1.0.0", []string{"linux"}, nil)

	sess := &fakeSession{installedVersions: nil}
	baseline := specsOf("com.platypus.sys-systemd-linux")
	if err := reconcileSystemPlugins(context.Background(), sess, "agent-1",
		baseline, "darwin", "amd64", os.DirFS(root)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	// Expect: list + 1 install (only sys-info; systemd-linux skipped on darwin).
	if len(sess.calls) != 2 {
		t.Fatalf("calls = %d (%+v); want 2", len(sess.calls), sess.calls)
	}
	if sess.calls[1].pluginID != "com.platypus.sys-info" {
		t.Errorf("install = %q; want sys-info only (linux plugin should skip)", sess.calls[1].pluginID)
	}
}

// TestReconcileSystemPlugins_PushesMatchingOSTarget is the positive
// counterpart: same plugin gets installed on a linux agent.
func TestReconcileSystemPlugins_PushesMatchingOSTarget(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "system-plugins")
	stagePublisherKey(t, root)
	stageBundlePlatformed(t, root, "com.platypus.sys-info", "2.0.0", nil, nil)
	stageBundlePlatformed(t, root, "com.platypus.sys-systemd-linux", "1.0.0", []string{"linux"}, nil)

	sess := &fakeSession{installedVersions: nil}
	baseline := specsOf("com.platypus.sys-systemd-linux")
	if err := reconcileSystemPlugins(context.Background(), sess, "agent-1",
		baseline, "linux", "amd64", os.DirFS(root)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	// list + 2 installs (sys-info + systemd-linux).
	if len(sess.calls) != 3 {
		t.Fatalf("calls = %d (%+v); want 3", len(sess.calls), sess.calls)
	}
}

// TestReconcileSystemPlugins_SkipsByArchTarget asserts arch filtering
// is independent of OS filtering.
func TestReconcileSystemPlugins_SkipsByArchTarget(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "system-plugins")
	stagePublisherKey(t, root)
	stageBundlePlatformed(t, root, "com.platypus.sys-info", "2.0.0", nil, nil)
	stageBundlePlatformed(t, root, "com.platypus.amd64-only", "1.0.0", nil, []string{"amd64"})

	sess := &fakeSession{installedVersions: nil}
	baseline := specsOf("com.platypus.amd64-only")
	if err := reconcileSystemPlugins(context.Background(), sess, "agent-1",
		baseline, "linux", "arm64", os.DirFS(root)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	// Only sys-info installs on the arm64 agent.
	if len(sess.calls) != 2 {
		t.Fatalf("calls = %d (%+v); want 2 (list + sys-info)", len(sess.calls), sess.calls)
	}
	if sess.calls[1].pluginID != "com.platypus.sys-info" {
		t.Errorf("install = %q; want sys-info only", sess.calls[1].pluginID)
	}
}

// TestReconcileSystemPlugins_EmptyAgentOSAllowsAll covers the
// "agent didn't report yet" race: empty agentOS means we can't
// filter, so we push everything (better than installing nothing
// during the fresh-enrol gap).
func TestReconcileSystemPlugins_EmptyAgentOSAllowsAll(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "system-plugins")
	stagePublisherKey(t, root)
	stageBundlePlatformed(t, root, "com.platypus.sys-info", "2.0.0", nil, nil)
	stageBundlePlatformed(t, root, "com.platypus.sys-systemd-linux", "1.0.0", []string{"linux"}, nil)

	sess := &fakeSession{installedVersions: nil}
	baseline := specsOf("com.platypus.sys-systemd-linux")
	if err := reconcileSystemPlugins(context.Background(), sess, "agent-1",
		baseline, "" /*agentOS unknown*/, "" /*agentArch unknown*/, os.DirFS(root)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(sess.calls) != 3 {
		t.Fatalf("calls = %d (%+v); want 3 — empty agent OS should not filter", len(sess.calls), sess.calls)
	}
}

// TestReconcileSystemPlugins_ForwardsConfigJsonFromHostSpecs: the
// rich-shape entry point picks up per-plugin config_overrides +
// granted_capabilities + schema_version from the host's stored
// PluginSpec rows and threads them into the wire request. This is
// the property that makes the whole rich-PluginSpec edifice
// load-bearing — without it, all the storage / API / enrollment
// plumbing in PR 3.1 would still be silently dropped at the
// agent-link reconciler.
func TestReconcileSystemPlugins_ForwardsConfigJsonFromHostSpecs(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "system-plugins")
	stagePublisherKey(t, root)
	stageBundle(t, root, "com.platypus.sys-info", "2.0.0", []string{"sysinfo"})
	stageBundle(t, root, "com.platypus.syslog-fwd", "1.0.0", []string{"net.dial"})

	specs := []storage.PluginSpec{
		{
			PluginID:            "com.platypus.syslog-fwd",
			GrantedCapabilities: []string{"net.dial"},
			ConfigOverrides:     []byte(`{"destination":"udp://10.0.0.1:514"}`),
			SchemaVersion:       1,
		},
	}
	sess := &fakeSession{installedVersions: nil}
	if err := reconcileSystemPlugins(
		context.Background(), sess, "agent-1", specs, "linux", "amd64",
		os.DirFS(filepath.Join(dataDir, "system-plugins")),
	); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	// Find the install call for syslog-fwd and assert the rich
	// fields all flowed through.
	var fwd *openCall
	for i := range sess.calls {
		if sess.calls[i].pluginID == "com.platypus.syslog-fwd" {
			fwd = &sess.calls[i]
			break
		}
	}
	if fwd == nil {
		t.Fatalf("no install call for syslog-fwd; calls=%+v", sess.calls)
	}
	if string(fwd.installConfigJSON) != `{"destination":"udp://10.0.0.1:514"}` {
		t.Fatalf("config_json = %s, want operator's overrides verbatim",
			fwd.installConfigJSON)
	}
	if fwd.installSchemaVersion != 1 {
		t.Fatalf("config_schema_version = %d, want 1", fwd.installSchemaVersion)
	}
	if len(fwd.installCaps) != 1 || fwd.installCaps[0] != "net.dial" {
		t.Fatalf("granted_capabilities = %v, want [net.dial] from spec",
			fwd.installCaps)
	}
}

// TestReconcileSystemPlugins_NoConfigStaysNoConfig: a host whose
// rich specs carry no config_overrides (the most common path —
// today's []string baseline lifted into minimal PluginSpec rows)
// produces install requests with empty config_json. The legacy
// install path stays bit-identical.
func TestReconcileSystemPlugins_NoConfigStaysNoConfig(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "system-plugins")
	stagePublisherKey(t, root)
	stageBundle(t, root, "com.platypus.sys-info", "2.0.0", []string{"sysinfo"})

	specs := []storage.PluginSpec{
		{PluginID: "com.platypus.sys-info"},
	}
	sess := &fakeSession{installedVersions: nil}
	if err := reconcileSystemPlugins(
		context.Background(), sess, "agent-1", specs, "linux", "amd64",
		os.DirFS(filepath.Join(dataDir, "system-plugins")),
	); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	for _, c := range sess.calls {
		if c.op == "install" && len(c.installConfigJSON) != 0 {
			t.Fatalf("install of %s carried config_json=%q; expected empty",
				c.pluginID, c.installConfigJSON)
		}
	}
}

func TestPlatformMatches(t *testing.T) {
	cases := []struct {
		name    string
		targets []string
		value   string
		want    bool
	}{
		{"empty targets matches anything", nil, "linux", true},
		{"empty value matches anything", []string{"linux"}, "", true},
		{"hit", []string{"linux", "darwin"}, "linux", true},
		{"miss", []string{"linux"}, "darwin", false},
		{"single match", []string{"darwin"}, "darwin", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := platformMatches(c.targets, c.value); got != c.want {
				t.Errorf("got %v; want %v", got, c.want)
			}
		})
	}
}
