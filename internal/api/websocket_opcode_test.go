package api

import "testing"

// TestTTYOpcodeClassify locks in the TTY WebSocket opcode contract.
//
// Only '0' (input) and '1' (resize) are accepted from the client. The legacy
// '{' opcode — a JSON-data alias for resize — is intentionally *not*
// supported, because a leading '{' byte collides with JSON produced by shell
// pipelines (e.g. `jq` or `curl ... | tee`). When that happened the server
// silently interpreted the data as a window-size update instead of forwarding
// it to stdin, losing bytes. Keeping this test prevents anyone re-adding '{'
// as an opcode without also solving that collision.
func TestTTYOpcodeClassify(t *testing.T) {
	cases := []struct {
		b    byte
		want ttyAction
	}{
		{'0', ttyActionInput},
		{'1', ttyActionResize},
		{'2', ttyActionIgnore}, // pause — raw shells don't support
		{'3', ttyActionIgnore}, // resume — raw shells don't support
		{'{', ttyActionUnknown},
		{'a', ttyActionUnknown},
		{0x00, ttyActionUnknown},
	}
	for _, tc := range cases {
		if got := classifyTTYOpcode(tc.b); got != tc.want {
			t.Errorf("classifyTTYOpcode(%q) = %v; want %v", tc.b, got, tc.want)
		}
	}
}
