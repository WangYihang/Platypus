package plugin

import (
	"sync"
	"time"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// logBuffer is a per-plugin bounded ring of log entries. Used by
// host_log to retain recent lines for the operator's "show me what
// this plugin printed last" view (PluginGetLogsRequest), without
// letting a chatty plugin grow unbounded memory.
//
// Entries older than `cap` are dropped silently. That's a deliberate
// trade-off: server-side persistence is the right place for permanent
// records (the activity recorder picks up every host-side line via
// log.L), and the in-memory buffer is just for the live "last N
// lines" panel.
type logBuffer struct {
	mu      sync.Mutex
	cap     int
	entries []logEntry
}

type logEntry struct {
	ts            time.Time
	level         string
	message       string
	correlationID string
}

func newLogBuffer(cap int) *logBuffer {
	if cap <= 0 {
		cap = 256
	}
	return &logBuffer{cap: cap}
}

func (b *logBuffer) append(ts time.Time, level, msg, corr string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.entries) >= b.cap {
		// Drop the oldest by sliding; cap stays small (a few hundred)
		// so the copy is cheap.
		copy(b.entries, b.entries[1:])
		b.entries = b.entries[:b.cap-1]
	}
	b.entries = append(b.entries, logEntry{ts: ts, level: level, message: msg, correlationID: corr})
}

// Tail returns the most recent up to n entries (n=0 means everything
// currently buffered) as proto-ready PluginLogEntry values.
func (b *logBuffer) Tail(n int) []*v2pb.PluginLogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	start := 0
	if n > 0 && len(b.entries) > n {
		start = len(b.entries) - n
	}
	out := make([]*v2pb.PluginLogEntry, 0, len(b.entries)-start)
	for _, e := range b.entries[start:] {
		out = append(out, &v2pb.PluginLogEntry{
			UnixNano:      e.ts.UnixNano(),
			Level:         e.level,
			Message:       e.message,
			CorrelationId: e.correlationID,
		})
	}
	return out
}
