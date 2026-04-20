package api

import (
	"testing"
	"time"
)

// TestTTYWebSocketLimits locks in the message-size and ping/pong config for
// /ws/:hash so a future refactor can't silently drop back to melody's 512-byte
// default. A 512-byte cap would sever a terminal session the moment a user
// pasted a modest line, and missing pings let firewall-idle connections hang.
func TestTTYWebSocketLimits(t *testing.T) {
	m := newTTYWebSocket()
	if m.Config.MaxMessageSize < 1<<20 {
		t.Errorf("TTY MaxMessageSize = %d; want >= 1 MiB", m.Config.MaxMessageSize)
	}
	if m.Config.PingPeriod <= 0 || m.Config.PingPeriod > 60*time.Second {
		t.Errorf("TTY PingPeriod = %v; want a positive value <= 60s", m.Config.PingPeriod)
	}
	if m.Config.PongWait <= m.Config.PingPeriod {
		t.Errorf("TTY PongWait (%v) must exceed PingPeriod (%v) so one missed pong doesn't trip the deadline", m.Config.PongWait, m.Config.PingPeriod)
	}
}

// TestNotifyWebSocketLimits locks in the /notify config. Events are small
// today but ensuring a non-default cap makes the contract explicit.
func TestNotifyWebSocketLimits(t *testing.T) {
	m := newNotifyWebSocket()
	if m.Config.MaxMessageSize < 64*1024 {
		t.Errorf("/notify MaxMessageSize = %d; want >= 64 KiB", m.Config.MaxMessageSize)
	}
	if m.Config.PingPeriod <= 0 {
		t.Errorf("/notify PingPeriod = %v; want a positive ping interval", m.Config.PingPeriod)
	}
}
