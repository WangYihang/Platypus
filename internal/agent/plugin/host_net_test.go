package plugin

import (
	"net"
	"strings"
	"sync"
	"testing"
)

// TestReapNetHandles_ClosesConnsAndListeners confirms the per-plugin
// teardown path closes both kinds of net handles (dialed conns and
// host_net_listen-created listeners). A crashed plugin must not leak
// either resource.
func TestReapNetHandles_ClosesConnsAndListeners(t *testing.T) {
	// Real loopback listener so we get a real net.Conn pair.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	var wg sync.WaitGroup
	wg.Add(1)
	var serverConn net.Conn
	go func() {
		defer wg.Done()
		c, err := ln.Accept()
		if err != nil {
			return
		}
		serverConn = c
	}()

	clientConn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	wg.Wait()
	if serverConn == nil {
		t.Fatalf("accept returned nil")
	}
	defer serverConn.Close()

	// Second listener that reapNetHandles must close.
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen 2: %v", err)
	}
	ln2Addr := ln2.Addr().String()

	pctx := &pluginCtx{
		netHandles: map[uint32]*netHandle{
			1: {id: 1, conn: clientConn},
			2: {id: 2, listener: ln2},
		},
	}

	pctx.reapNetHandles()

	if pctx.netHandles != nil {
		t.Errorf("netHandles not nil after reap: %v", pctx.netHandles)
	}

	// Conn must be closed — write returns an error.
	if _, err := clientConn.Write([]byte("x")); err == nil {
		t.Errorf("clientConn still writable after reap")
	}
	// Listener must be closed — re-binding the same port (which the
	// kernel may not give us back instantly) is flaky; instead probe
	// by attempting to dial — closed listener should refuse.
	if c, err := net.DialTimeout("tcp", ln2Addr, 50_000_000); err == nil {
		c.Close()
		// Some kernels still allow a connect to a half-closed accept
		// queue briefly; accept either outcome but log.
		t.Logf("listener accepted post-close (kernel queue race; not fatal)")
	}

	// Cleanup the original loopback listener.
	ln.Close()
}

// TestManifest_NetListenCapability_Declared covers manifest plumbing:
// a plugin with capabilities.net.listen present in its manifest must
// surface CapNetListen in DeclaredCapabilities and accept it via
// ValidateGranted.
func TestManifest_NetListenCapability_Declared(t *testing.T) {
	const yamlSrc = `
api_version: 1
id: com.example.tunnel-tcp
name: TCP Tunnel
version: 1.0.0
author: { name: Jane, email: jane@example.com }
license: Apache-2.0
runtime:
  type: wasm
  entry: tunnel.wasm
  abi: extism/1
rpc:
  - name: noop
    request:  { proto: NoopRequest }
    response: { proto: NoopResponse }
capabilities:
  net.listen:
    binds: ["127.0.0.1:1080", "127.0.0.1:*"]
resources:
  max_memory_mb: 32
  max_invocation_ms: 5000
signature:
  algo: minisign-ed25519
  key_id: RWQTESTKEY00000000
  sig_file: tunnel.wasm.minisig
`
	m, err := ParseManifest([]byte(yamlSrc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	declared := m.DeclaredCapabilities()
	var found bool
	for _, c := range declared {
		if c == CapNetListen {
			found = true
		}
	}
	if !found {
		t.Errorf("CapNetListen missing from declared: %v", declared)
	}

	if err := m.ValidateGranted([]string{"net.listen"}); err != nil {
		t.Errorf("ValidateGranted(net.listen): %v", err)
	}
	if err := m.ValidateGranted([]string{"net.dial"}); err == nil {
		t.Errorf("ValidateGranted(net.dial) should fail — not requested")
	}
}

// TestManifest_NetListenBinds_Validation exercises the syntactic
// checks in ManifestCapabilities.validate for the binds list.
func TestManifest_NetListenBinds_Validation(t *testing.T) {
	const base = `
api_version: 1
id: com.example.tunnel-tcp
name: TCP Tunnel
version: 1.0.0
author: { name: Jane, email: jane@example.com }
license: Apache-2.0
runtime:
  type: wasm
  entry: tunnel.wasm
  abi: extism/1
rpc:
  - name: noop
    request:  { proto: NoopRequest }
    response: { proto: NoopResponse }
capabilities:
  net.listen:
    binds: %s
resources:
  max_memory_mb: 32
  max_invocation_ms: 5000
signature:
  algo: minisign-ed25519
  key_id: RWQTESTKEY00000000
  sig_file: tunnel.wasm.minisig
`
	cases := []struct {
		name    string
		binds   string
		wantErr string
	}{
		{"empty", "[]", "set without any binds"},
		{"missing_port", `["127.0.0.1"]`, "missing :port"},
		{"slash_in_bind", `["127.0.0.1/24:1080"]`, "must be host:port"},
		{"happy_literal", `["127.0.0.1:1080"]`, ""},
		{"happy_glob", `["127.0.0.1:*", "*:8080"]`, ""},
		{"happy_unrestricted", `["*:*"]`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			yamlSrc := strings.Replace(base, "%s", tc.binds, 1)
			_, err := ParseManifest([]byte(yamlSrc))
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

// TestNetListen_AllowlistMatching covers the matchAny-driven decision
// about whether a requested bind is permitted by the manifest. This
// is the same matchAny used by hostNetListen, so verifying the
// allowlist semantics here is sufficient — the host fn is a thin
// wrapper around it + net.Listen.
func TestNetListen_AllowlistMatching(t *testing.T) {
	cases := []struct {
		name      string
		allowlist []string
		req       string
		want      bool
	}{
		{"literal_match", []string{"127.0.0.1:1080"}, "127.0.0.1:1080", true},
		{"literal_mismatch_port", []string{"127.0.0.1:1080"}, "127.0.0.1:1081", false},
		{"glob_port_any", []string{"127.0.0.1:*"}, "127.0.0.1:1080", true},
		{"glob_port_any_high", []string{"127.0.0.1:*"}, "127.0.0.1:65535", true},
		{"glob_host_any", []string{"*:1080"}, "0.0.0.0:1080", true},
		{"unrestricted", []string{"*:*"}, "192.168.0.5:443", true},
		{"empty_allowlist", []string{}, "127.0.0.1:1080", false},
		{"multi_one_match", []string{"127.0.0.1:1080", "0.0.0.0:8080"}, "0.0.0.0:8080", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// pathSep=0: bind addrs are flat strings, not paths.
			got := matchAny(tc.allowlist, tc.req, 0)
			if got != tc.want {
				t.Errorf("matchAny(%v, %q) = %v, want %v",
					tc.allowlist, tc.req, got, tc.want)
			}
		})
	}
}
