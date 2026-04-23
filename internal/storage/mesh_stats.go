package storage

import (
	"context"
	"database/sql"
	"time"
)

// MeshLinkStat is one row in the mesh_link_stats time-series table.
// Counters are cumulative across the link's life; the downsampler
// turns them into rates when serving history requests.
type MeshLinkStat struct {
	At        time.Time
	ProjectID string
	NodeA     string
	NodeB     string
	BytesIn   int64
	BytesOut  int64
	MsgsIn    int64
	MsgsOut   int64
	RTTNs     *int64
}

// MachineStat is one row in the machine_stats time-series table.
type MachineStat struct {
	At         time.Time
	HostID     string
	ProjectID  string
	CPUPercent *float64
	MemPercent *float64
}

// MeshStatsRepo is the storage handle for both topology time-series
// tables. They share access patterns (point-in-time insert, range
// query, GC-by-age) so they live together.
type MeshStatsRepo struct{ db *sql.DB }

// MeshStats exposes the repo. Matches the other `db.Xxx()` helpers.
func (db *DB) MeshStats() *MeshStatsRepo { return &MeshStatsRepo{db: db.DB} }

// InsertLinkStats writes a batch of link samples in one transaction.
// Empty slice is a no-op. Inserts ignore duplicates on the primary
// key (ts, project_id, node_a, node_b) — the 1 Hz coalescer in core
// guarantees monotonic timestamps, but retries after a crash or
// clock skew shouldn't crash the writer.
func (r *MeshStatsRepo) InsertLinkStats(ctx context.Context, rows []MeshLinkStat) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO mesh_link_stats
		  (ts, project_id, node_a, node_b, bytes_in, bytes_out, msgs_in, msgs_out, rtt_ns)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	for _, row := range rows {
		a, b := canonicalPair(row.NodeA, row.NodeB)
		var rtt interface{}
		if row.RTTNs != nil {
			rtt = *row.RTTNs
		}
		if _, err := stmt.ExecContext(ctx, row.At.UTC(), row.ProjectID, a, b,
			row.BytesIn, row.BytesOut, row.MsgsIn, row.MsgsOut, rtt); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	rollback = false
	return nil
}

// InsertMachineStats writes a batch of machine samples.
func (r *MeshStatsRepo) InsertMachineStats(ctx context.Context, rows []MachineStat) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO machine_stats
		  (ts, host_id, project_id, cpu_percent, mem_percent)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	for _, row := range rows {
		var cpu, mem interface{}
		if row.CPUPercent != nil {
			cpu = *row.CPUPercent
		}
		if row.MemPercent != nil {
			mem = *row.MemPercent
		}
		if _, err := stmt.ExecContext(ctx, row.At.UTC(), row.HostID, row.ProjectID, cpu, mem); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	rollback = false
	return nil
}

// LinkHistoryOpts narrows ListLinkHistory. MaxPoints, when > 0, asks
// the repo to downsample the result server-side so the frontend
// always receives a bounded number of points. The repo uses a simple
// modulo-based thinning: every Nth row where N = total / MaxPoints.
type LinkHistoryOpts struct {
	Since     time.Time
	Until     time.Time
	MaxPoints int
}

// ListLinkHistory returns the rows for a single undirected edge
// within the given window, oldest first. Canonicalises node order
// before querying so callers don't have to.
func (r *MeshStatsRepo) ListLinkHistory(ctx context.Context, projectID, nodeA, nodeB string, opts LinkHistoryOpts) ([]MeshLinkStat, error) {
	a, b := canonicalPair(nodeA, nodeB)
	rows, err := r.db.QueryContext(ctx, `
		SELECT ts, project_id, node_a, node_b, bytes_in, bytes_out, msgs_in, msgs_out, rtt_ns
		  FROM mesh_link_stats
		 WHERE project_id = ? AND node_a = ? AND node_b = ?
		   AND ts >= ? AND ts <= ?
		 ORDER BY ts ASC`,
		projectID, a, b, opts.Since.UTC(), opts.Until.UTC())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []MeshLinkStat
	for rows.Next() {
		var s MeshLinkStat
		var rtt sql.NullInt64
		if err := rows.Scan(&s.At, &s.ProjectID, &s.NodeA, &s.NodeB,
			&s.BytesIn, &s.BytesOut, &s.MsgsIn, &s.MsgsOut, &rtt); err != nil {
			return nil, err
		}
		if rtt.Valid {
			v := rtt.Int64
			s.RTTNs = &v
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return thin(out, opts.MaxPoints), nil
}

// MachineHistoryOpts narrows ListMachineHistory, same shape as
// LinkHistoryOpts.
type MachineHistoryOpts = LinkHistoryOpts

// ListMachineHistory returns CPU/memory samples for a single host in
// the given window, oldest first.
func (r *MeshStatsRepo) ListMachineHistory(ctx context.Context, hostID string, opts MachineHistoryOpts) ([]MachineStat, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT ts, host_id, project_id, cpu_percent, mem_percent
		  FROM machine_stats
		 WHERE host_id = ? AND ts >= ? AND ts <= ?
		 ORDER BY ts ASC`,
		hostID, opts.Since.UTC(), opts.Until.UTC())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []MachineStat
	for rows.Next() {
		var s MachineStat
		var cpu, mem sql.NullFloat64
		if err := rows.Scan(&s.At, &s.HostID, &s.ProjectID, &cpu, &mem); err != nil {
			return nil, err
		}
		if cpu.Valid {
			v := cpu.Float64
			s.CPUPercent = &v
		}
		if mem.Valid {
			v := mem.Float64
			s.MemPercent = &v
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return thinMachine(out, opts.MaxPoints), nil
}

// GCOlderThan deletes rows from both tables older than cutoff.
// Returns the total row count removed across both tables.
func (r *MeshStatsRepo) GCOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	c := cutoff.UTC()
	a, err := r.db.ExecContext(ctx, `DELETE FROM mesh_link_stats WHERE ts < ?`, c)
	if err != nil {
		return 0, err
	}
	na, _ := a.RowsAffected()
	b, err := r.db.ExecContext(ctx, `DELETE FROM machine_stats WHERE ts < ?`, c)
	if err != nil {
		return na, err
	}
	nb, _ := b.RowsAffected()
	return na + nb, nil
}

func canonicalPair(a, b string) (string, string) {
	if a > b {
		return b, a
	}
	return a, b
}

// thin returns at most max rows from src, evenly spaced. max<=0 means
// no thinning. Preserves first and last so charts don't lose their
// endpoints.
func thin(src []MeshLinkStat, max int) []MeshLinkStat {
	if max <= 0 || len(src) <= max {
		return src
	}
	out := make([]MeshLinkStat, 0, max)
	step := float64(len(src)-1) / float64(max-1)
	for i := 0; i < max; i++ {
		out = append(out, src[int(float64(i)*step)])
	}
	return out
}

func thinMachine(src []MachineStat, max int) []MachineStat {
	if max <= 0 || len(src) <= max {
		return src
	}
	out := make([]MachineStat, 0, max)
	step := float64(len(src)-1) / float64(max-1)
	for i := 0; i < max; i++ {
		out = append(out, src[int(float64(i)*step)])
	}
	return out
}
