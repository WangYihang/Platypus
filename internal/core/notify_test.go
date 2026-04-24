package core

import (
	"encoding/json"
	"testing"
)

// Notify events are shipped as a tiny JSON envelope {type, data} so the
// frontend can dispatch on type without inspecting payload shape.
// Pinning the shape here keeps the wire format stable across refactors.
func TestBuildNotifyMessage_EnvelopeShape(t *testing.T) {
	raw, err := BuildNotifyMessage("host.seen", map[string]string{
		"project_id": "prj-1",
		"host_id":    "host-abc",
	})
	if err != nil {
		t.Fatalf("BuildNotifyMessage: %v", err)
	}

	var env struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Type != "host.seen" {
		t.Errorf("type = %q; want host.seen", env.Type)
	}
	var body struct {
		ProjectID string `json:"project_id"`
		HostID    string `json:"host_id"`
	}
	if err := json.Unmarshal(env.Data, &body); err != nil {
		t.Fatalf("data unmarshal: %v", err)
	}
	if body.ProjectID != "prj-1" || body.HostID != "host-abc" {
		t.Errorf("data = %+v", body)
	}
}

// Known event constants cover the set of lifecycle messages the client
// subscribes to. Pinning them in a test guards against silent typos
// (e.g. "host.seen" vs "host_seen") that would desync the two sides.
func TestNotifyEventConstants(t *testing.T) {
	for _, tc := range []struct{ got, want string }{
		{EventHostSeen, "host.seen"},
		{EventSessionOpened, "session.opened"},
		{EventSessionClosed, "session.closed"},
	} {
		if tc.got != tc.want {
			t.Errorf("event const = %q; want %q", tc.got, tc.want)
		}
	}
}

// BroadcastNotify without a NotifyWebSocket attached is a no-op — a
// server started without the RESTful subsystem (hypothetical) should
// not crash when core code tries to emit events.
func TestBroadcastNotify_NilWebSocket_NoPanic(t *testing.T) {
	// Don't touch Ctx.NotifyWebSocket — it starts nil in NewApp.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("broadcast panicked: %v", r)
		}
	}()
	// We need a Ctx at least, even if NotifyWebSocket is nil.
	if Ctx == nil {
		t.Skip("Ctx not initialised by another test")
	}
	BroadcastNotify(EventHostSeen, map[string]string{"x": "y"})
}
