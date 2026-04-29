-- 000023_host_versions.up.sql — record per-host build identity and
-- wire-protocol version. Replaces the single `agent_version` column
-- with a structured set of fields that serve two distinct concerns:
--
--   build_version / build_commit / build_date — informational.
--     Surfaced in the host list so operators can answer "exactly which
--     binary is on which box" without SSHing in. build_version is
--     semver (pkg/version.Version); build_commit is the short git SHA;
--     build_date is RFC3339.
--
--   protocol_version — semantic. Single monotonic uint32 that the
--     server compares against MinSupportedProtocolVersion at enroll
--     time and (in a follow-up pass) uses to drive auto-upgrade
--     pushes for hosts running stale wire protocols. NULL means the
--     agent didn't advertise a version (pre-versioning binary); the
--     server treats those as v1 for compatibility decisions.
--
-- agent_version is dropped outright — internal-development phase, no
-- external consumers, and its semantics were ambiguous (sometimes
-- "v2", sometimes a build identifier, never a real version).
--
-- Note: build_commit is named with a build_ prefix to keep clear of
-- SQLite's reserved word `commit`. The Go struct field remains
-- `Commit` and the protobuf field is `commit`; the rename is a
-- column-only concern.

ALTER TABLE hosts DROP COLUMN agent_version;

ALTER TABLE hosts ADD COLUMN build_version TEXT;
ALTER TABLE hosts ADD COLUMN build_commit TEXT;
ALTER TABLE hosts ADD COLUMN build_date TEXT;
ALTER TABLE hosts ADD COLUMN protocol_version INTEGER;
