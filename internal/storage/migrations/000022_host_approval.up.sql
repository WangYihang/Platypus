-- 000022_host_approval.up.sql — gate fresh agent enrollments behind
-- explicit admin approval. Closes the "leaked install token = silent
-- mesh entry" gap: an attacker who exfiltrates a PAT can still redeem
-- it (the cert chain proves nothing about admin intent), but the
-- resulting host shows up as `pending_approval` and can't open a link
-- until an operator clicks Approve.
--
-- approval_status is the host-level state machine:
--
--   pending  — fresh enrollment awaiting admin decision (default)
--   approved — admin accepted; agent can run normally
--   rejected — admin denied; cert is revoked-by-policy and link rejects
--
-- approval_decided_at / _by stamp the decision for audit; reason is an
-- optional free-form note the operator can drop into the Reject/Approve
-- dialog ("kim's laptop, expected", "wrong PAT, decline").
--
-- enrollment_tokens.auto_approve lets admins mint a "pre-authorized"
-- PAT for automation — the resulting host enrolls straight to
-- `approved` with no human in the loop. Mirrors Tailscale's
-- --preauthorized auth-key flag. Defaults to 0 (manual review required)
-- so the safer behaviour wins by default; the mint dialog has to opt
-- in explicitly.

ALTER TABLE hosts ADD COLUMN approval_status TEXT NOT NULL DEFAULT 'pending'
    CHECK (approval_status IN ('pending', 'approved', 'rejected'));
ALTER TABLE hosts ADD COLUMN approval_decided_at DATETIME;
ALTER TABLE hosts ADD COLUMN approval_decided_by TEXT;
ALTER TABLE hosts ADD COLUMN approval_reason TEXT;

ALTER TABLE enrollment_tokens ADD COLUMN auto_approve INTEGER NOT NULL DEFAULT 0
    CHECK (auto_approve IN (0, 1));

-- install_download_tokens carries auto_approve too: when an admin
-- mints an install link with "skip approval", the PAT minted at
-- consume time inherits the flag, and the host that redeems it
-- enrolls directly to `approved`. Default 0 keeps the strict
-- behaviour for existing rows.
ALTER TABLE install_download_tokens ADD COLUMN auto_approve INTEGER NOT NULL DEFAULT 0
    CHECK (auto_approve IN (0, 1));

-- Per-project approval-list query: WHERE project_id = ? AND
-- approval_status = 'pending' ORDER BY first_seen_at DESC. Index
-- supports the "Pending approvals" badge count + drawer list.
CREATE INDEX idx_hosts_approval_pending
    ON hosts(project_id, first_seen_at DESC)
 WHERE approval_status = 'pending';
