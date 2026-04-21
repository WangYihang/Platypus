-- Index for ListForProject — the dashboard and SessionsPage both query
-- "all sessions for project P, newest first, optionally since T". Without
-- this index SQLite scans every session row for the project.
CREATE INDEX IF NOT EXISTS idx_sessions_project_connected
  ON sessions(project_id, connected_at DESC);
