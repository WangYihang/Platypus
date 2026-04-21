-- Reverse of 000001_init.up.sql. Order matters: drop children before parents
-- so the FK references don't block the drop even with foreign_keys = ON.

DROP INDEX IF EXISTS idx_sessions_live;
DROP INDEX IF EXISTS idx_sessions_listener;
DROP INDEX IF EXISTS idx_sessions_host;

DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS hosts;
DROP TABLE IF EXISTS listeners;
DROP TABLE IF EXISTS project_members;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS users;
