-- 000007_topology_stats.up.sql — persistent time-series tables for the
-- Topology visualisation.
--
-- mesh_link_stats stores per-edge traffic counters, one row per second
-- when a link is observed. Counters are cumulative from the link's
-- creation; the UI / downsampler computes rate = delta / dt.
--
-- machine_stats stores per-host CPU / memory samples derived from the
-- agent-pushed SysInfoResponse.
--
-- Both tables are intentionally narrow so row size stays small on
-- SQLite. Indexes cover the two access patterns the history API uses:
--   - list a single link / host time-window, newest first
--   - GC by age (WHERE ts < ?)

CREATE TABLE mesh_link_stats (
    ts            DATETIME NOT NULL,
    project_id    TEXT NOT NULL,
    node_a        TEXT NOT NULL,
    node_b        TEXT NOT NULL,
    bytes_in      INTEGER NOT NULL,
    bytes_out     INTEGER NOT NULL,
    msgs_in       INTEGER NOT NULL,
    msgs_out      INTEGER NOT NULL,
    rtt_ns        INTEGER,
    PRIMARY KEY (ts, project_id, node_a, node_b)
);

CREATE INDEX idx_mesh_link_stats_edge_ts
    ON mesh_link_stats(project_id, node_a, node_b, ts DESC);

CREATE INDEX idx_mesh_link_stats_gc
    ON mesh_link_stats(ts);

CREATE TABLE machine_stats (
    ts            DATETIME NOT NULL,
    host_id       TEXT NOT NULL,
    project_id    TEXT NOT NULL,
    cpu_percent   REAL,
    mem_percent   REAL,
    PRIMARY KEY (ts, host_id)
);

CREATE INDEX idx_machine_stats_host_ts
    ON machine_stats(host_id, ts DESC);

CREATE INDEX idx_machine_stats_gc
    ON machine_stats(ts);
