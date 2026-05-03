package plugin

import "testing"

// parseWasmHandler is the marker-string parser the agent's stream
// dispatcher uses to tell wasm-mediated streams (manifest's
// host_handler starts with "wasm:") apart from claim-only legacy
// providers ("agent.X" / any other registered name). Tests live
// here because the parsing rules are stable + cheap to assert; a
// regression in this helper would silently misroute every stream.

func TestParseWasmHandler(t *testing.T) {
	cases := []struct {
		in     string
		method string
		isWasm bool
	}{
		// Happy path: "wasm:" prefix + non-empty method name.
		{"wasm:file_read", "file_read", true},
		{"wasm:process_open", "process_open", true},
		{"wasm:a", "a", true},
		// Legacy claim-only markers — host fn names registered via
		// SetStreamProvider. Must NOT be misclassified as wasm.
		{"agent.file_read", "", false},
		{"agent.process", "", false},
		// Edge cases the parser must reject as "not wasm" so the
		// dispatcher falls through to the claim-only path:
		{"wasm:", "", false},      // empty method — useless
		{"wasm", "", false},       // missing colon
		{"wasms:foo", "", false},  // typo prefix
		{":file_read", "", false}, // empty prefix
		{"", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			method, isWasm := parseWasmHandler(tc.in)
			if isWasm != tc.isWasm {
				t.Errorf("parseWasmHandler(%q) isWasm=%v, want %v",
					tc.in, isWasm, tc.isWasm)
			}
			if method != tc.method {
				t.Errorf("parseWasmHandler(%q) method=%q, want %q",
					tc.in, method, tc.method)
			}
		})
	}
}
