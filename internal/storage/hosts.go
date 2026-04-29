package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Host mirrors a row in the hosts table. FingerprintFallback is true when
// the agent didn't report a platform-level machine_id and the server is
// aggregating sessions purely on hostname + sorted MACs. The UI surfaces
// that flag so operators know the merge may be lossy.
type Host struct {
	ID                  string
	ProjectID           string
	MachineID           string // "" when FingerprintFallback=true
	Fingerprint         string
	FingerprintFallback bool
	Hostname            string
	PrimaryAlias        string
	OS                  string
	FirstSeenAt         time.Time
	LastSeenAt          time.Time

	// --- Rich agent-reported system info (all optional). Populated
	// on enroll and refreshed on every agent reconnect. The UI uses
	// these to show "what's on the box" even when the agent is
	// offline. ---
	AgentID         string
	Arch            string
	Platform        string
	PlatformFamily  string
	PlatformVersion string
	KernelVersion   string
	CPUModel        string
	NumCPU          int
	MemTotalBytes   int64
	CurrentUser     string
	Timezone        string
	PrimaryIP       string
	PrimaryMAC      string
	BootTimeUnix    int64

	// Build identity (migration 000023). Populated from the agent's
	// pkg/version at enroll time and refreshed on every SysInfo
	// reconnect. All advisory; never gates security decisions.
	// BuildVersion is semver, Commit is short git SHA, BuildDate is
	// RFC3339. ProtocolVersion is a separate monotonic uint32 used
	// for capability gating — see internal/link.ProtocolVersion.
	BuildVersion    string
	Commit          string
	BuildDate       string
	ProtocolVersion uint32

	// --- Hardware / chassis classification (migration 000012). All
	// optional; MachineType is the high-level category surfaced in
	// the hosts list ("container" / "vm" / "bare_metal" / "laptop" /
	// "desktop" / "unknown"). GPUSummary is a short
	// "NVIDIA RTX 4090; Intel UHD 770" blurb built server-side from
	// the first few entries of the live GPU list.
	MachineType   string
	ChassisType   string
	ProductVendor string
	ProductName   string
	BIOSVendor    string
	BIOSVersion   string
	GPUSummary    string

	// --- Admin approval gate (migration 000022). Every fresh
	// enrollment lands as "pending" unless the redeemed PAT had its
	// auto_approve flag set; admins flip the state via the
	// /hosts/:hid/approve|reject endpoints. Link handler refuses WS
	// upgrades for non-approved hosts so a leaked PAT can't reach the
	// agent runtime even after a successful enroll. ---
	ApprovalStatus    HostApprovalStatus
	ApprovalDecidedAt *time.Time
	ApprovalDecidedBy string
	ApprovalReason    string
}

// HostApprovalStatus is the host-level approval state. See migration
// 000022 for the lifecycle semantics.
type HostApprovalStatus string

const (
	HostApprovalPending  HostApprovalStatus = "pending"
	HostApprovalApproved HostApprovalStatus = "approved"
	HostApprovalRejected HostApprovalStatus = "rejected"
)

// HostIdentity carries the agent-reported identity we upsert into the
// hosts table. SeenAt is used for both first_seen_at (on insert) and
// last_seen_at (always). Rich fields mirror Host and are optional —
// callers leave them zero when only the minimal identity is known.
type HostIdentity struct {
	ProjectID   string
	MachineID   string
	Fingerprint string
	Hostname    string
	OS          string
	SeenAt      time.Time

	AgentID         string
	Arch            string
	Platform        string
	PlatformFamily  string
	PlatformVersion string
	KernelVersion   string
	CPUModel        string
	NumCPU          int
	MemTotalBytes   int64
	CurrentUser     string
	Timezone        string
	PrimaryIP       string
	PrimaryMAC      string
	BootTimeUnix    int64

	BuildVersion    string
	Commit          string
	BuildDate       string
	ProtocolVersion uint32

	MachineType   string
	ChassisType   string
	ProductVendor string
	ProductName   string
	BIOSVendor    string
	BIOSVersion   string
	GPUSummary    string

	// InitialApproval is the approval_status to stamp on a fresh
	// hosts row only — Upsert never overwrites an existing row's
	// approval state. Empty defaults to HostApprovalPending so a
	// caller that forgets to set it still gets the safe behaviour.
	InitialApproval HostApprovalStatus
}

func (db *DB) Hosts() *HostRepo { return &HostRepo{db: db.DB} }

type HostRepo struct {
	db *sql.DB
}

// hostAllCols is the canonical ordered projection used by every
// scanHost* helper. Change it in one place and every query that
// feeds into scanHostRow keeps working.
const hostAllCols = `id, project_id, machine_id, fingerprint, fingerprint_fallback,
       hostname, primary_alias, os, first_seen_at, last_seen_at,
       agent_id, arch, platform, platform_family, platform_version,
       kernel_version, cpu_model, num_cpu, mem_total_bytes,
       current_user, timezone, primary_ip, primary_mac,
       boot_time_unix,
       build_version, build_commit, build_date, protocol_version,
       machine_type, chassis_type, product_vendor, product_name,
       bios_vendor, bios_version, gpu_summary,
       approval_status, approval_decided_at, approval_decided_by, approval_reason`

// Upsert merges the given identity into the hosts table. Matching order:
//
//  1. If ident carries an AgentID and a row with that agent_id exists, update it.
//  2. Else if (project_id, machine_id) exists and machine_id != "", update it.
//  3. Else if (project_id, fingerprint) exists, update it — and if the new
//     identity has a non-empty machine_id, backfill it and clear
//     fingerprint_fallback.
//  4. Else insert a new row.
//
// Returns the resulting Host, always with FirstSeenAt preserved across
// updates and LastSeenAt set to SeenAt.
func (r *HostRepo) Upsert(ctx context.Context, ident *HostIdentity) (*Host, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()

	var existing *Host

	// 1) Lookup by agent_id (preferred for v2 link reconnects where
	// the server already knows the issuing cert's agent id).
	if ident.AgentID != "" {
		existing, err = scanHostSingle(tx.QueryRowContext(ctx,
			`SELECT `+hostAllCols+` FROM hosts WHERE agent_id = ?`,
			ident.AgentID))
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	// 2) Lookup by machine_id.
	if existing == nil && ident.MachineID != "" {
		existing, err = scanHostSingle(tx.QueryRowContext(ctx,
			`SELECT `+hostAllCols+` FROM hosts WHERE project_id = ? AND machine_id = ?`,
			ident.ProjectID, ident.MachineID))
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	// 3) Lookup by fingerprint as fallback / upgrade path.
	if existing == nil {
		existing, err = scanHostSingle(tx.QueryRowContext(ctx,
			`SELECT `+hostAllCols+` FROM hosts WHERE project_id = ? AND fingerprint = ?`,
			ident.ProjectID, ident.Fingerprint))
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	if existing != nil {
		// Update the row in place. For optional rich fields we take
		// the new value when non-zero but otherwise keep the existing
		// one — a reconnecting agent that can't re-collect (e.g.
		// gopsutil denied /proc) shouldn't wipe historical info.
		newMachineID := existing.MachineID
		newFallback := existing.FingerprintFallback
		if ident.MachineID != "" {
			newMachineID = ident.MachineID
			newFallback = false
		}
		merged := mergeHost(existing, ident)
		merged.MachineID = newMachineID
		merged.FingerprintFallback = newFallback
		merged.Hostname = ident.Hostname
		merged.OS = coalesceString(ident.OS, existing.OS)
		merged.Fingerprint = ident.Fingerprint
		merged.LastSeenAt = ident.SeenAt.UTC()

		if _, err := tx.ExecContext(ctx, `
			UPDATE hosts
			   SET machine_id = ?, fingerprint = ?, fingerprint_fallback = ?,
			       hostname = ?, os = ?, last_seen_at = ?,
			       agent_id = ?, arch = ?, platform = ?, platform_family = ?,
			       platform_version = ?, kernel_version = ?, cpu_model = ?,
			       num_cpu = ?, mem_total_bytes = ?, current_user = ?,
			       timezone = ?, primary_ip = ?, primary_mac = ?,
			       boot_time_unix = ?,
			       build_version = ?, build_commit = ?, build_date = ?,
			       protocol_version = ?,
			       machine_type = ?, chassis_type = ?, product_vendor = ?,
			       product_name = ?, bios_vendor = ?, bios_version = ?,
			       gpu_summary = ?
			 WHERE id = ?`,
			nullIfEmpty(merged.MachineID), merged.Fingerprint, merged.FingerprintFallback,
			merged.Hostname, merged.OS, merged.LastSeenAt,
			nullIfEmpty(merged.AgentID), nullIfEmpty(merged.Arch), nullIfEmpty(merged.Platform),
			nullIfEmpty(merged.PlatformFamily), nullIfEmpty(merged.PlatformVersion),
			nullIfEmpty(merged.KernelVersion), nullIfEmpty(merged.CPUModel),
			nullIfInt(merged.NumCPU), nullIfInt64(merged.MemTotalBytes),
			nullIfEmpty(merged.CurrentUser), nullIfEmpty(merged.Timezone),
			nullIfEmpty(merged.PrimaryIP), nullIfEmpty(merged.PrimaryMAC),
			nullIfInt64(merged.BootTimeUnix),
			nullIfEmpty(merged.BuildVersion), nullIfEmpty(merged.Commit),
			nullIfEmpty(merged.BuildDate), nullIfUint32(merged.ProtocolVersion),
			nullIfEmpty(merged.MachineType), nullIfEmpty(merged.ChassisType),
			nullIfEmpty(merged.ProductVendor), nullIfEmpty(merged.ProductName),
			nullIfEmpty(merged.BIOSVendor), nullIfEmpty(merged.BIOSVersion),
			nullIfEmpty(merged.GPUSummary),
			existing.ID,
		); err != nil {
			return nil, err
		}
		rollback = false
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return merged, nil
	}

	// 4) Insert fresh. Approval defaults to pending unless caller
	// said otherwise (e.g. PAT with auto_approve=true).
	approval := ident.InitialApproval
	if approval == "" {
		approval = HostApprovalPending
	}
	h := &Host{
		ID:                  uuid.NewString(),
		ProjectID:           ident.ProjectID,
		MachineID:           ident.MachineID,
		Fingerprint:         ident.Fingerprint,
		FingerprintFallback: ident.MachineID == "",
		Hostname:            ident.Hostname,
		OS:                  ident.OS,
		FirstSeenAt:         ident.SeenAt.UTC(),
		LastSeenAt:          ident.SeenAt.UTC(),
		AgentID:             ident.AgentID,
		Arch:                ident.Arch,
		Platform:            ident.Platform,
		PlatformFamily:      ident.PlatformFamily,
		PlatformVersion:     ident.PlatformVersion,
		KernelVersion:       ident.KernelVersion,
		CPUModel:            ident.CPUModel,
		NumCPU:              ident.NumCPU,
		MemTotalBytes:       ident.MemTotalBytes,
		CurrentUser:         ident.CurrentUser,
		Timezone:            ident.Timezone,
		PrimaryIP:           ident.PrimaryIP,
		PrimaryMAC:          ident.PrimaryMAC,
		BootTimeUnix:        ident.BootTimeUnix,
		BuildVersion:        ident.BuildVersion,
		Commit:              ident.Commit,
		BuildDate:           ident.BuildDate,
		ProtocolVersion:     ident.ProtocolVersion,
		MachineType:         ident.MachineType,
		ChassisType:         ident.ChassisType,
		ProductVendor:       ident.ProductVendor,
		ProductName:         ident.ProductName,
		BIOSVendor:          ident.BIOSVendor,
		BIOSVersion:         ident.BIOSVersion,
		GPUSummary:          ident.GPUSummary,
		ApprovalStatus:      approval,
	}
	// auto_approve=true on the PAT → admin/system already decided at
	// mint time; stamp the decision so the audit trail is uniform.
	if approval == HostApprovalApproved {
		decidedAt := ident.SeenAt.UTC()
		h.ApprovalDecidedAt = &decidedAt
		h.ApprovalDecidedBy = "system:auto-approve"
	}
	var decidedAt any
	if h.ApprovalDecidedAt != nil {
		decidedAt = *h.ApprovalDecidedAt
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO hosts
		  (id, project_id, machine_id, fingerprint, fingerprint_fallback,
		   hostname, primary_alias, os, first_seen_at, last_seen_at,
		   agent_id, arch, platform, platform_family, platform_version,
		   kernel_version, cpu_model, num_cpu, mem_total_bytes,
		   current_user, timezone, primary_ip, primary_mac,
		   boot_time_unix,
		   build_version, build_commit, build_date, protocol_version,
		   machine_type, chassis_type, product_vendor, product_name,
		   bios_vendor, bios_version, gpu_summary,
		   approval_status, approval_decided_at, approval_decided_by, approval_reason)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		        ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		        ?, ?, ?, ?, ?,
		        ?, ?, ?, ?, ?, ?, ?,
		        ?, ?, ?, ?)`,
		h.ID, h.ProjectID, nullIfEmpty(h.MachineID), h.Fingerprint,
		h.FingerprintFallback, h.Hostname, h.PrimaryAlias, h.OS,
		h.FirstSeenAt, h.LastSeenAt,
		nullIfEmpty(h.AgentID), nullIfEmpty(h.Arch), nullIfEmpty(h.Platform),
		nullIfEmpty(h.PlatformFamily), nullIfEmpty(h.PlatformVersion),
		nullIfEmpty(h.KernelVersion), nullIfEmpty(h.CPUModel),
		nullIfInt(h.NumCPU), nullIfInt64(h.MemTotalBytes),
		nullIfEmpty(h.CurrentUser), nullIfEmpty(h.Timezone),
		nullIfEmpty(h.PrimaryIP), nullIfEmpty(h.PrimaryMAC),
		nullIfInt64(h.BootTimeUnix),
		nullIfEmpty(h.BuildVersion), nullIfEmpty(h.Commit),
		nullIfEmpty(h.BuildDate), nullIfUint32(h.ProtocolVersion),
		nullIfEmpty(h.MachineType), nullIfEmpty(h.ChassisType),
		nullIfEmpty(h.ProductVendor), nullIfEmpty(h.ProductName),
		nullIfEmpty(h.BIOSVendor), nullIfEmpty(h.BIOSVersion),
		nullIfEmpty(h.GPUSummary),
		string(h.ApprovalStatus), decidedAt,
		nullIfEmpty(h.ApprovalDecidedBy), nullIfEmpty(h.ApprovalReason),
	); err != nil {
		return nil, err
	}
	rollback = false
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return h, nil
}

// mergeHost returns a copy of existing with optional fields overwritten
// by any non-zero values in ident. Fields carrying identity (ID,
// ProjectID, FirstSeenAt) are never changed here.
func mergeHost(existing *Host, ident *HostIdentity) *Host {
	h := *existing
	if ident.AgentID != "" {
		h.AgentID = ident.AgentID
	}
	if ident.Arch != "" {
		h.Arch = ident.Arch
	}
	if ident.Platform != "" {
		h.Platform = ident.Platform
	}
	if ident.PlatformFamily != "" {
		h.PlatformFamily = ident.PlatformFamily
	}
	if ident.PlatformVersion != "" {
		h.PlatformVersion = ident.PlatformVersion
	}
	if ident.KernelVersion != "" {
		h.KernelVersion = ident.KernelVersion
	}
	if ident.CPUModel != "" {
		h.CPUModel = ident.CPUModel
	}
	if ident.NumCPU != 0 {
		h.NumCPU = ident.NumCPU
	}
	if ident.MemTotalBytes != 0 {
		h.MemTotalBytes = ident.MemTotalBytes
	}
	if ident.CurrentUser != "" {
		h.CurrentUser = ident.CurrentUser
	}
	if ident.Timezone != "" {
		h.Timezone = ident.Timezone
	}
	if ident.PrimaryIP != "" {
		h.PrimaryIP = ident.PrimaryIP
	}
	if ident.PrimaryMAC != "" {
		h.PrimaryMAC = ident.PrimaryMAC
	}
	if ident.BootTimeUnix != 0 {
		h.BootTimeUnix = ident.BootTimeUnix
	}
	if ident.BuildVersion != "" {
		h.BuildVersion = ident.BuildVersion
	}
	if ident.Commit != "" {
		h.Commit = ident.Commit
	}
	if ident.BuildDate != "" {
		h.BuildDate = ident.BuildDate
	}
	if ident.ProtocolVersion != 0 {
		h.ProtocolVersion = ident.ProtocolVersion
	}
	if ident.MachineType != "" {
		h.MachineType = ident.MachineType
	}
	if ident.ChassisType != "" {
		h.ChassisType = ident.ChassisType
	}
	if ident.ProductVendor != "" {
		h.ProductVendor = ident.ProductVendor
	}
	if ident.ProductName != "" {
		h.ProductName = ident.ProductName
	}
	if ident.BIOSVendor != "" {
		h.BIOSVendor = ident.BIOSVendor
	}
	if ident.BIOSVersion != "" {
		h.BIOSVersion = ident.BIOSVersion
	}
	if ident.GPUSummary != "" {
		h.GPUSummary = ident.GPUSummary
	}
	return &h
}

func coalesceString(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (r *HostRepo) ListByProject(ctx context.Context, projectID string) ([]*Host, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+hostAllCols+`
		  FROM hosts WHERE project_id = ?
		 ORDER BY hostname ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []*Host{}
	for rows.Next() {
		h, err := scanHostRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (r *HostRepo) GetByID(ctx context.Context, id string) (*Host, error) {
	h, err := scanHostSingle(r.db.QueryRowContext(ctx, `
		SELECT `+hostAllCols+` FROM hosts WHERE id = ?`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return h, nil
}

// GetByAgentID looks up a host by its associated agent_id. Returns
// ErrNotFound when no row has claimed that agent (yet) — callers
// typically fall back to the Upsert path.
func (r *HostRepo) GetByAgentID(ctx context.Context, agentID string) (*Host, error) {
	h, err := scanHostSingle(r.db.QueryRowContext(ctx, `
		SELECT `+hostAllCols+` FROM hosts WHERE agent_id = ?`, agentID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return h, nil
}

// TouchLastSeen bumps last_seen_at for the host identified by
// agent_id. Cheap one-row UPDATE; the agent-link handler runs this on
// a heartbeat tick so the Web UI's online window (60s lookback over
// last_seen_at) reflects long-lived links accurately. Returns
// ErrNotFound when no row claims the agent_id yet — callers should
// treat that as transient (the enroll-time Upsert may still be in
// flight) and let the next tick try again.
func (r *HostRepo) TouchLastSeen(ctx context.Context, agentID string, at time.Time) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE hosts SET last_seen_at = ? WHERE agent_id = ?`,
		at.UTC(), agentID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// scanHostRow scans a single row from *sql.Rows or *sql.Row via rowScanner.
func scanHostRow(s rowScanner) (*Host, error) {
	var (
		h               Host
		machineID       sql.NullString
		primary         sql.NullString
		agentID         sql.NullString
		arch            sql.NullString
		platform        sql.NullString
		platformFamily  sql.NullString
		platformVersion sql.NullString
		kernelVersion   sql.NullString
		cpuModel        sql.NullString
		numCPU          sql.NullInt64
		memTotalBytes   sql.NullInt64
		currentUser     sql.NullString
		timezone        sql.NullString
		primaryIP       sql.NullString
		primaryMAC      sql.NullString
		bootTimeUnix    sql.NullInt64
		buildVersion    sql.NullString
		buildCommit     sql.NullString
		buildDate       sql.NullString
		protocolVersion sql.NullInt64
		machineType     sql.NullString
		chassisType     sql.NullString
		productVendor   sql.NullString
		productName     sql.NullString
		biosVendor      sql.NullString
		biosVersion     sql.NullString
		gpuSummary      sql.NullString
		approvalStatus  string
		approvalAt      sql.NullTime
		approvalBy      sql.NullString
		approvalReason  sql.NullString
	)
	err := s.Scan(
		&h.ID, &h.ProjectID, &machineID, &h.Fingerprint, &h.FingerprintFallback,
		&h.Hostname, &primary, &h.OS, &h.FirstSeenAt, &h.LastSeenAt,
		&agentID, &arch, &platform, &platformFamily, &platformVersion,
		&kernelVersion, &cpuModel, &numCPU, &memTotalBytes,
		&currentUser, &timezone, &primaryIP, &primaryMAC,
		&bootTimeUnix,
		&buildVersion, &buildCommit, &buildDate, &protocolVersion,
		&machineType, &chassisType, &productVendor, &productName,
		&biosVendor, &biosVersion, &gpuSummary,
		&approvalStatus, &approvalAt, &approvalBy, &approvalReason,
	)
	if err != nil {
		return nil, err
	}
	if machineID.Valid {
		h.MachineID = machineID.String
	}
	if primary.Valid {
		h.PrimaryAlias = primary.String
	}
	if agentID.Valid {
		h.AgentID = agentID.String
	}
	if arch.Valid {
		h.Arch = arch.String
	}
	if platform.Valid {
		h.Platform = platform.String
	}
	if platformFamily.Valid {
		h.PlatformFamily = platformFamily.String
	}
	if platformVersion.Valid {
		h.PlatformVersion = platformVersion.String
	}
	if kernelVersion.Valid {
		h.KernelVersion = kernelVersion.String
	}
	if cpuModel.Valid {
		h.CPUModel = cpuModel.String
	}
	if numCPU.Valid {
		h.NumCPU = int(numCPU.Int64)
	}
	if memTotalBytes.Valid {
		h.MemTotalBytes = memTotalBytes.Int64
	}
	if currentUser.Valid {
		h.CurrentUser = currentUser.String
	}
	if timezone.Valid {
		h.Timezone = timezone.String
	}
	if primaryIP.Valid {
		h.PrimaryIP = primaryIP.String
	}
	if primaryMAC.Valid {
		h.PrimaryMAC = primaryMAC.String
	}
	if bootTimeUnix.Valid {
		h.BootTimeUnix = bootTimeUnix.Int64
	}
	if buildVersion.Valid {
		h.BuildVersion = buildVersion.String
	}
	if buildCommit.Valid {
		h.Commit = buildCommit.String
	}
	if buildDate.Valid {
		h.BuildDate = buildDate.String
	}
	if protocolVersion.Valid {
		h.ProtocolVersion = uint32(protocolVersion.Int64)
	}
	if machineType.Valid {
		h.MachineType = machineType.String
	}
	if chassisType.Valid {
		h.ChassisType = chassisType.String
	}
	if productVendor.Valid {
		h.ProductVendor = productVendor.String
	}
	if productName.Valid {
		h.ProductName = productName.String
	}
	if biosVendor.Valid {
		h.BIOSVendor = biosVendor.String
	}
	if biosVersion.Valid {
		h.BIOSVersion = biosVersion.String
	}
	if gpuSummary.Valid {
		h.GPUSummary = gpuSummary.String
	}
	h.ApprovalStatus = HostApprovalStatus(approvalStatus)
	if approvalAt.Valid {
		v := approvalAt.Time
		h.ApprovalDecidedAt = &v
	}
	if approvalBy.Valid {
		h.ApprovalDecidedBy = approvalBy.String
	}
	if approvalReason.Valid {
		h.ApprovalReason = approvalReason.String
	}
	return &h, nil
}

// scanHostSingle returns (nil, sql.ErrNoRows) when the row is empty, so
// Upsert can distinguish "no match" from "scan failure".
func scanHostSingle(row *sql.Row) (*Host, error) {
	h, err := scanHostRow(row)
	if err != nil {
		return nil, err
	}
	return h, nil
}

// nullIfEmpty maps "" to a NULL value so UNIQUE(project_id, machine_id)
// doesn't collide across multiple fingerprint-only rows (SQLite treats
// NULLs as distinct in UNIQUE constraints).
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Approve transitions a host from `pending` (or even `rejected`) to
// `approved` and stamps the decision metadata. Idempotent on rows
// already approved by the same user — re-approval is a no-op write.
// Returns ErrNotFound when the id doesn't match any row.
func (r *HostRepo) Approve(ctx context.Context, id, byUser, reason string, at time.Time) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE hosts
		   SET approval_status = 'approved',
		       approval_decided_at = ?,
		       approval_decided_by = ?,
		       approval_reason = ?
		 WHERE id = ?`,
		at.UTC(), byUser, nullIfEmpty(reason), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Reject transitions a host to `rejected`. Same shape as Approve.
// Callers should also revoke any cert linked to this host so the
// agent can't re-link by simply retrying — Reject only updates the
// approval flag.
func (r *HostRepo) Reject(ctx context.Context, id, byUser, reason string, at time.Time) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE hosts
		   SET approval_status = 'rejected',
		       approval_decided_at = ?,
		       approval_decided_by = ?,
		       approval_reason = ?
		 WHERE id = ?`,
		at.UTC(), byUser, nullIfEmpty(reason), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListPendingByProject returns every host in a project still awaiting
// admin approval, oldest first so the operator works through the
// queue in arrival order. Drives the per-project approval drawer
// + the badge count on the top-bar.
func (r *HostRepo) ListPendingByProject(ctx context.Context, projectID string) ([]*Host, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+hostAllCols+`
		  FROM hosts
		 WHERE project_id = ? AND approval_status = 'pending'
		 ORDER BY first_seen_at ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []*Host{}
	for rows.Next() {
		h, err := scanHostRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// CountPendingByProject is a cheap COUNT(*) for the pending-approvals
// badge — avoids transferring all the rich-info columns when the UI
// only wants a number.
func (r *HostRepo) CountPendingByProject(ctx context.Context, projectID string) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM hosts
		 WHERE project_id = ? AND approval_status = 'pending'`, projectID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// nullIfInt / nullIfInt64 do the same for numeric columns — a zero
// means "not reported", which we want to persist as NULL rather than
// as a genuine 0 value.
func nullIfInt(n int) any {
	if n == 0 {
		return nil
	}
	return int64(n)
}

func nullIfInt64(n int64) any {
	if n == 0 {
		return nil
	}
	return n
}

// nullIfUint32 mirrors nullIfInt for unsigned 32-bit columns. Zero is
// a meaningful "not reported" sentinel for protocol_version (genuine
// versions start at 1), so we fold it to NULL rather than persisting
// it as a real zero.
func nullIfUint32(n uint32) any {
	if n == 0 {
		return nil
	}
	return int64(n)
}
