-- LLM-generated one-line summaries for recordings.
--
-- `terminal_recordings.summary` is populated asynchronously after
-- Session.Finish persists the row; NULL means "not yet generated"
-- (or the project opted out, or the LLM call failed). The UI
-- renders summaries when present and falls through to metadata-
-- only otherwise — clients MUST tolerate NULL.
ALTER TABLE terminal_recordings ADD COLUMN summary TEXT;

-- Per-project opt-in for the LLM round-trip. Default OFF: cast
-- files can contain pasted secrets, so we never send them out
-- without the project owner explicitly enabling it.
ALTER TABLE projects ADD COLUMN ai_summaries_enabled INTEGER NOT NULL DEFAULT 0
    CHECK (ai_summaries_enabled IN (0, 1));
