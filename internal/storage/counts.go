package storage

import (
	"context"
	"time"
)

// Counts is the server-wide telemetry roll-up the status-bar
// endpoint surfaces. Cross-project — every project on this server
// contributes — so operators see "is the whole server healthy" at
// a glance, not per-project.
//
// LiveHosts uses an `onlineWindow` parameter rather than a const so
// the same threshold the UI uses (60s by default in lib/time.ts)
// stays the source of truth and isn't duplicated server-side.
type Counts struct {
	// Hosts is the total host row count across every project.
	Hosts int
	// LiveHosts is the count of hosts whose last_seen_at falls
	// within `onlineWindow` of now (UTC).
	LiveHosts int
	// Sessions is the historical session row count — live + closed.
	Sessions int
	// LiveSessions is the count of session rows with no
	// disconnected_at stamp.
	LiveSessions int
}

// Counts returns the server-wide telemetry roll-up. Designed to be
// cheap enough to call once per status-bar refresh tick (1 Hz):
// each subquery is a single COUNT(*) over an indexed column, the
// table sizes are bounded, and we do all four in a single round trip
// via the SQLite UNION ALL pattern.
func (db *DB) Counts(ctx context.Context, onlineWindow time.Duration) (Counts, error) {
	cutoff := time.Now().UTC().Add(-onlineWindow)

	rows, err := db.DB.QueryContext(ctx, `
		SELECT 'hosts',         COUNT(*) FROM hosts
		UNION ALL
		SELECT 'live_hosts',    COUNT(*) FROM hosts WHERE last_seen_at > ?
		UNION ALL
		SELECT 'sessions',      COUNT(*) FROM sessions
		UNION ALL
		SELECT 'live_sessions', COUNT(*) FROM sessions WHERE disconnected_at IS NULL
	`, cutoff)
	if err != nil {
		return Counts{}, err
	}
	defer rows.Close()

	var out Counts
	for rows.Next() {
		var key string
		var n int
		if err := rows.Scan(&key, &n); err != nil {
			return Counts{}, err
		}
		switch key {
		case "hosts":
			out.Hosts = n
		case "live_hosts":
			out.LiveHosts = n
		case "sessions":
			out.Sessions = n
		case "live_sessions":
			out.LiveSessions = n
		}
	}
	return out, rows.Err()
}
