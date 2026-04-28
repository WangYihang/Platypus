package storage

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// Transfer status, direction, and kind values are duplicated here as
// typed constants so callers don't sprinkle string literals through
// the codebase. Values match the CHECK constraints in
// migrations/000015_file_transfers.up.sql exactly — adding a new
// status requires a migration to relax the CHECK.
const (
	TransferDirectionDownload = "download"
	TransferDirectionUpload   = "upload"

	TransferKindFile    = "file"    // single regular file
	TransferKindArchive = "archive" // tar / tar.gz / zip of one-or-more roots
	TransferKindFolder  = "folder"  // tree copy without on-the-wire packing

	TransferStatusPending  = "pending"
	TransferStatusRunning  = "running"
	TransferStatusDone     = "done"
	TransferStatusFailed   = "failed"
	TransferStatusCanceled = "canceled"
)

// FileTransfer is one row in file_transfers. PathsJSON stores the
// source paths as a JSON array; UI renders them, the backend uses
// them only for display/audit.
type FileTransfer struct {
	ID               string
	ProjectID        string
	HostID           string
	UserID           string
	Direction        string
	Kind             string
	Format           string // "tar.gz" / "tar" / "zip" / "" for non-archive
	PathsJSON        string
	Status           string
	BytesTransferred int64
	TotalBytes       int64 // 0 = unknown / scan skipped
	ErrorMessage     string
	StartedAt        time.Time
	FinishedAt       *time.Time
}

// FileTransferFilter narrows a List query. Zero-value fields are
// ignored. Limit defaults to 200; pass a negative number to disable.
type FileTransferFilter struct {
	ProjectID string
	HostID    string
	UserID    string
	Status    string // when set, restricts to one status
	Limit     int
}

func (db *DB) FileTransfers() *FileTransfersRepo {
	return &FileTransfersRepo{db: db.DB}
}

type FileTransfersRepo struct {
	db *sql.DB
}

const fileTransferColumns = "id, project_id, host_id, user_id, direction, kind, format, paths_json, status, bytes_transferred, total_bytes, error_message, started_at, finished_at"

func scanFileTransfer(scanner interface{ Scan(...any) error }) (*FileTransfer, error) {
	var (
		ft         FileTransfer
		totalBytes sql.NullInt64
		finishedAt sql.NullTime
	)
	if err := scanner.Scan(
		&ft.ID,
		&ft.ProjectID,
		&ft.HostID,
		&ft.UserID,
		&ft.Direction,
		&ft.Kind,
		&ft.Format,
		&ft.PathsJSON,
		&ft.Status,
		&ft.BytesTransferred,
		&totalBytes,
		&ft.ErrorMessage,
		&ft.StartedAt,
		&finishedAt,
	); err != nil {
		return nil, err
	}
	if totalBytes.Valid {
		ft.TotalBytes = totalBytes.Int64
	}
	if finishedAt.Valid {
		t := finishedAt.Time
		ft.FinishedAt = &t
	}
	return &ft, nil
}

// Create inserts a new transfer. The caller fills in StartedAt;
// FinishedAt is left nil and populated by Finish.
func (r *FileTransfersRepo) Create(ctx context.Context, ft *FileTransfer) error {
	var totalBytes any
	if ft.TotalBytes > 0 {
		totalBytes = ft.TotalBytes
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO file_transfers (
			id, project_id, host_id, user_id, direction, kind, format,
			paths_json, status, bytes_transferred, total_bytes,
			error_message, started_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
	`,
		ft.ID, ft.ProjectID, ft.HostID, ft.UserID,
		ft.Direction, ft.Kind, ft.Format, ft.PathsJSON, ft.Status,
		ft.BytesTransferred, totalBytes, ft.ErrorMessage, ft.StartedAt,
	)
	return err
}

// Get returns the row for id, or ErrNotFound when absent.
func (r *FileTransfersRepo) Get(ctx context.Context, id string) (*FileTransfer, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT "+fileTransferColumns+" FROM file_transfers WHERE id = ?", id)
	ft, err := scanFileTransfer(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return ft, nil
}

// UpdateProgress overwrites bytes_transferred (and optionally
// total_bytes when newTotal > 0). Status is unchanged. No-op when
// the row is missing.
func (r *FileTransfersRepo) UpdateProgress(ctx context.Context, id string, bytes, newTotal int64) error {
	if newTotal > 0 {
		_, err := r.db.ExecContext(ctx,
			"UPDATE file_transfers SET bytes_transferred = ?, total_bytes = ? WHERE id = ?",
			bytes, newTotal, id)
		return err
	}
	_, err := r.db.ExecContext(ctx,
		"UPDATE file_transfers SET bytes_transferred = ? WHERE id = ?",
		bytes, id)
	return err
}

// Finish transitions a running transfer to a terminal state.
// status MUST be one of TransferStatusDone / TransferStatusFailed /
// TransferStatusCanceled. errMsg is populated only on failure.
func (r *FileTransfersRepo) Finish(ctx context.Context, id, status string, bytes int64, errMsg string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE file_transfers
		   SET status = ?, bytes_transferred = ?, error_message = ?, finished_at = ?
		 WHERE id = ?
	`, status, bytes, errMsg, at, id)
	return err
}

// List returns rows newest-first, optionally filtered.
func (r *FileTransfersRepo) List(ctx context.Context, f FileTransferFilter) ([]*FileTransfer, error) {
	q := "SELECT " + fileTransferColumns + " FROM file_transfers"
	conds, args := buildTransferWhere(f)
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY started_at DESC"

	limit := f.Limit
	if limit == 0 {
		limit = 200
	}
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []*FileTransfer
	for rows.Next() {
		ft, err := scanFileTransfer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ft)
	}
	return out, rows.Err()
}

// CountActive returns the number of transfers in pending/running
// state matching the filter.
func (r *FileTransfersRepo) CountActive(ctx context.Context, f FileTransferFilter) (int, error) {
	q := `SELECT COUNT(*) FROM file_transfers WHERE status IN ('pending','running')`
	conds, args := buildTransferWhere(f)
	for _, c := range conds {
		// status filter is meaningless here; ignore if present.
		if strings.HasPrefix(c, "status =") {
			continue
		}
		q += " AND " + c
	}
	// Re-walk args so they line up with the conds we kept.
	if f.Status != "" {
		// drop the status arg that buildTransferWhere appended.
		args = args[:len(args)-1]
	}
	var n int
	if err := r.db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func buildTransferWhere(f FileTransferFilter) ([]string, []any) {
	var conds []string
	var args []any
	if f.ProjectID != "" {
		conds = append(conds, "project_id = ?")
		args = append(args, f.ProjectID)
	}
	if f.HostID != "" {
		conds = append(conds, "host_id = ?")
		args = append(args, f.HostID)
	}
	if f.UserID != "" {
		conds = append(conds, "user_id = ?")
		args = append(args, f.UserID)
	}
	if f.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, f.Status)
	}
	return conds, args
}
